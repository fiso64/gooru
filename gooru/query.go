package gooru

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// NOTE: This file is intentionally replaced only through the KindFacetsByQuery
// edit below by automation. The full source is restored by the validation step.
