package ledger

import _ "embed"

//go:embed schema_v1.sql
var schemaV1 string

//go:embed schema_v2.sql
var schemaV2 string

//go:embed schema_v3.sql
var schemaV3 string
