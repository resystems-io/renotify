// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import _ "embed"

//go:embed schema_v1.sql
var schemaV1 string

//go:embed schema_v2.sql
var schemaV2 string

//go:embed schema_v3.sql
var schemaV3 string
