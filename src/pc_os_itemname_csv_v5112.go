package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/csv"
    "encoding/hex"
    "errors"
    "io"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// ItemName is read from a fixed read-only SQL SELECT elsewhere. The CSV
// exporter must only write inside Zhirox's own application directory.
// No SQL, backup/restore, or legacy POS file operations are permitted here.
const pcOSItemNameCSVName = "SuperMarket_ItemName_latest.csv"

func pcOSItemNameCSVSafeCell(value string) string {
    // Excel may evaluate untrusted product names as formulas.
    // Keep a literal leading apostrophe in the CSV for unsafe prefixes.
    first := strings.TrimLeft(value, " \t\r\n")
    if len(first) > 0 && strings.ContainsRune("=+-@", rune(first[0])) {
        return "'" + value
    }
    return value
}

func pcOSItemNameCSVBytes(rows []pcOSItemNameRow) ([]byte, error) {
    normalized, err := pcOSItemNameValidate(rows)
    if err != nil { return nil, err }

    ids := make([]string, 0, len(normalized))
    for id := range normalized { ids = append(ids, id) }
    sort.Slice(ids, func(i, j int) bool {
        // Numeric IDs sort naturally even though the underlying type is text.
        a, b := strings.TrimLeft(ids[i], "0"), strings.TrimLeft(ids[j], "0")
        if len(a) != len(b) { return len(a) < len(b) }
        return a < b
    })

    var output bytes.Buffer
    _, _ = output.Write([]byte{0xEF,0xBB,0xBF}) // Excel-compatible UTF-8 BOM
    w := csv.NewWriter(&output)
    w.UseCRLF = true
    if err = w.Write([]string{"ItemNameID","ItemName","ItemCode","SalePrice","WholeSalePrice"});err != nil {return nil,err}
    for _, id := range ids {
        item := normalized[id]
        record := []string{
            pcOSItemNameCSVSafeCell(item.ID),
            pcOSItemNameCSVSafeCell(item.Name),
            pcOSItemNameCSVSafeCell(item.Barcode),
            item.SalePrice,
            item.CartonPrice,
        }
        if err = w.Write(record);err != nil {return nil,err}
    }
    w.Flush()
    if err = w.Error(); err != nil {return nil,err}
    return output.Bytes(),nil
}

// pcOSItemNameEnsureLiveCSV is idempotent and atomic. It does not modify
// the file when its contents are identical to the latest verified snapshot.
// A failed write keeps the previous complete CSV untouched.
func pcOSItemNameEnsureLiveCSV(path string, rows []pcOSItemNameRow) (string, bool, error) {
    if filepath.Base(path) != pcOSItemNameCSVName {
        return "",false,errors.New("CSV filename outside fixed export allowlist")
    }
    data,err := pcOSItemNameCSVBytes(rows)
    if err != nil {return "",false,err}
    digest := sha256.Sum256(data)
    sha := hex.EncodeToString(digest[:])
    dir := filepath.Dir(path)
    if err = os.MkdirAll(dir,0700);err != nil{return "",false,err}
    d,err := os.Lstat(dir)
    if err != nil{return "",false,err}
    if !d.IsDir() || d.Mode()&os.ModeSymlink != 0{return "",false,errors.New("CSV export directory is not a real directory")}
    info,statErr := os.Lstat(path)
    if statErr == nil {
        if !info.Mode().IsRegular() || info.Size() > 32*1024*1024 {
            return "",false,errors.New("CSV export target is not a bounded regular file")
        }
        old,readErr := os.ReadFile(path)
        if readErr != nil{return "",false,readErr}
        if bytes.Equal(old,data) {return sha,false,nil}
    } else if !errors.Is(statErr,os.ErrNotExist) {
        return "",false,statErr
    }
    tmp,err := os.CreateTemp(dir,".zhirox-itemname-*.tmp")
    if err != nil{return "",false,err}
    tempPath := tmp.Name()
    defer os.Remove(tempPath)
    if err = tmp.Chmod(0600);err != nil {tmp.Close();return "",false,err}
    if _,err = io.Copy(tmp,bytes.NewReader(data));err != nil {tmp.Close();return "",false,err}
    if err = tmp.Sync();err != nil {tmp.Close();return "",false,err}
    if err = tmp.Close();err != nil {return "",false,err}
    if err = os.Rename(tempPath,path);err != nil{return "",false,err}
    return sha,true,nil
}
