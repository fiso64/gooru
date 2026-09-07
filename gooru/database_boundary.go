package gooru

import "gooru.local/internal/database"

// databaseTx and contentTagPair keep database implementation types behind the
// core package's composition boundary. Business-logic files should depend on
// these package-local names rather than importing internal/database directly.
type databaseTx = database.Tx
type contentTagPair = database.ContentTagPair
type tagFacetExclusion = database.TagFacetExclusion

var errInvalidSavedSearchOrder = database.ErrInvalidSavedSearchOrder
