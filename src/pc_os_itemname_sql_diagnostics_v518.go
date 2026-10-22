package main

import (
 "errors"
 "strings"
)

// Fail closed and return only diagnostic categories. sqlcmd (ODBC) is known to
// print diagnostic text on stdout on some Windows versions even with -r 1.
// Never copy stdout/stderr, SQL statements, product rows or credentials into
// a remote result. Inspect at most a few prefixed diagnostic lines.
func pcOSItemNameSQLCmdDiagnostic(stdout []string, stderr string, exitErr error) error {
 if exitErr == nil { return nil }
 var lines []string
 if len(stderr) > 4096 {stderr=stderr[:4096]}
 lines=append(lines,stderr)
 for _,line:=range stdout {
  if len(lines)>=25 {break}
  item:=strings.TrimSpace(line)
  if len(item)>2048 {continue}
  lower:=strings.ToLower(item)
  if strings.HasPrefix(lower,"sqlcmd:") ||
     strings.HasPrefix(lower,"msg ") ||
     strings.HasPrefix(lower,"sqlstate") ||
     strings.HasPrefix(lower,"microsoft (r) sqlcmd") {
    lines=append(lines,item)
  }
 }
 e:=strings.ToLower(strings.Join(lines,"\n"))
 classification:="SQLCMD_ERROR_UNCLASSIFIED"
 switch {
 case strings.Contains(e,"invalid option") ||
      strings.Contains(e,"unknown option") ||
      strings.Contains(e,"unrecognized option") ||
      strings.Contains(e,"unexpected argument") ||
      strings.Contains(e,"unrecognized argument") ||
      strings.Contains(e,"option not recognized") ||
      strings.Contains(e,"invalid command line argument") ||
      strings.Contains(e,"unrecognized command line option"):
  classification="SQLCMD_UNSUPPORTED_OPTION"
 case strings.Contains(e,"target principal name") ||
      strings.Contains(e,"hostname in certificate") ||
      strings.Contains(e,"certificate name mismatch"):
  classification="SQL_TLS_CERT_NAME_MISMATCH"
 case strings.Contains(e,"certificate chain was issued by an authority that is not trusted") ||
      strings.Contains(e,"certificate verify failed") ||
      strings.Contains(e,"self-signed certificate"):
  classification="SQL_TLS_CERT_TRUST"
 case strings.Contains(e,"ssl provider") ||
      strings.Contains(e,"tls handshake") ||
      strings.Contains(e,"handshake failure") ||
      strings.Contains(e,"protocol version"):
  classification="SQL_TLS_HANDSHAKE"
 case strings.Contains(e,"login failed") ||
      strings.Contains(e,"18456"):
  classification="SQL_INTEGRATED_LOGIN_FAILED"
 case strings.Contains(e,"cannot open database") ||
      strings.Contains(e,"4060"):
  classification="SQL_DATABASE_ACCESS_DENIED"
 case strings.Contains(e,"network-related") ||
      strings.Contains(e,"server was not found") ||
      strings.Contains(e,"08001") ||
      strings.Contains(e,"connection timeout"):
  classification="SQL_CONNECTION_FAILED"
 case strings.Contains(e,"invalid column name") ||
      strings.Contains(e,"invalid object name") ||
      strings.Contains(e,"permission denied") ||
      strings.Contains(e,"permission was denied"):
  classification="SQL_SCHEMA_OR_PERMISSION_ERROR"
 }
 return errors.New("read-only ItemName SQLCMD failed: "+classification)
}
