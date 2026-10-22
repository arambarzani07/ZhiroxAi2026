package main

import (
 "errors"
 "strings"
)

// Only return a fixed diagnostic category to the relay. Do not forward
// stdout/stderr, SQL text, rows, product names, barcodes or auth details.
// The SQLCMD process uses -r 1, which directs diagnostic messages to stderr.
func pcOSItemNameSQLCmdDiagnostic(stdout []string, stderr string, exitErr error) error {
 if exitErr == nil { return nil }
 _ = stdout // Explicitly discarded: business rows may begin with "Error:".
 e := strings.ToLower(stderr)
 classification := "SQLCMD_ERROR_UNCLASSIFIED"
 switch {
 case strings.Contains(e, "target principal name") ||
      strings.Contains(e, "hostname in certificate") ||
      strings.Contains(e, "certificate name mismatch"):
  classification = "SQL_TLS_CERT_NAME_MISMATCH"
 case strings.Contains(e, "certificate chain was issued by an authority that is not trusted") ||
      strings.Contains(e, "certificate verify failed") ||
      strings.Contains(e, "self-signed certificate"):
  classification = "SQL_TLS_CERT_TRUST"
 case strings.Contains(e, "ssl provider") ||
      strings.Contains(e, "tls handshake") ||
      strings.Contains(e, "handshake failure") ||
      strings.Contains(e, "protocol version"):
  classification = "SQL_TLS_HANDSHAKE"
 case strings.Contains(e, "login failed") ||
      strings.Contains(e, "18456"):
  classification = "SQL_INTEGRATED_LOGIN_FAILED"
 case strings.Contains(e, "cannot open database") ||
      strings.Contains(e, "4060"):
  classification = "SQL_DATABASE_ACCESS_DENIED"
 case strings.Contains(e, "network-related") ||
      strings.Contains(e, "server was not found") ||
      strings.Contains(e, "08001") ||
      strings.Contains(e, "connection timeout"):
  classification = "SQL_CONNECTION_FAILED"
 case strings.Contains(e, "invalid column name") ||
      strings.Contains(e, "invalid object name") ||
      strings.Contains(e, "permission denied") ||
      strings.Contains(e, "permission was denied"):
  classification = "SQL_SCHEMA_OR_PERMISSION_ERROR"
 }
 return errors.New("read-only ItemName SQLCMD failed: " + classification)
}
