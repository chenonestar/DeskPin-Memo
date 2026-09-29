package store

import (
	"database/sql"
	"encoding/json"
	"testing"
)

type sqlTx = sql.Tx

func mustJSON(t *testing.T, d *Dump) []byte {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustParse(t *testing.T, b []byte) *Dump {
	t.Helper()
	var d Dump
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}
