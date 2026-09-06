from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected snippet missing in {path}")
    p.write_text(text.replace(old, new, 1))


replace(
    "frontend/src/lib/queries/library.ts",
    "  suggestions: (scope: number, q: string) => ['library', 'suggestions', scope, q] as const",
    "  suggestions: (scope: number, q: string, existing: string) => ['library', 'suggestions', scope, q, existing] as const",
)
replace(
    "frontend/src/lib/queries/library.ts",
    """  return createQuery(() => ({
    queryKey: libraryKeys.suggestions(getAuthScope(), getDraft()),
    enabled: getAuthenticated() && getDraft().trim().length > 0,
    queryFn: ({ signal }) => new ApiClient().searchSuggestions(getDraft().trim(), 10, getExisting(), signal),
    staleTime: 30_000
  }));""",
    """  return createQuery(() => {
    const scope = getAuthScope();
    const draft = getDraft().trim();
    const existing = getExisting().trim();
    return {
      queryKey: libraryKeys.suggestions(scope, draft, existing),
      enabled: getAuthenticated() && draft.length > 0,
      queryFn: ({ signal }) => new ApiClient().searchSuggestions(draft, 10, existing, signal),
      placeholderData: (previousData) => previousData,
      staleTime: 30_000
    };
  });""",
)

replace(
    "frontend/src/lib/state/libraryWorkflow.svelte.ts",
    "  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;\n",
    "",
)
replace(
    "frontend/src/lib/state/libraryWorkflow.svelte.ts",
    """  function setSearch(value: string) {
    searchDraft.set(value);
    if (searchDebounce) clearTimeout(searchDebounce);
    if (suggestionDebounce) clearTimeout(suggestionDebounce);
    searchDebounce = setTimeout(submitSearch, 280);
    suggestionDebounce = setTimeout(() => suggestionSearch.set(value.trim()), 160);
  }

  function setSearchDraft(value: string) {
    if (suggestionDebounce) clearTimeout(suggestionDebounce);
    suggestionDebounce = setTimeout(() => suggestionSearch.set(value.trim()), 160);
  }

  function commitSearch(value: string) {
    if (searchDebounce) clearTimeout(searchDebounce);
    if (suggestionDebounce) clearTimeout(suggestionDebounce);""",
    """  function setSearch(value: string) {
    searchDraft.set(value);
    suggestionSearch.set(value.trim());
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(submitSearch, 280);
  }

  function setSearchDraft(value: string) {
    suggestionSearch.set(value.trim());
  }

  function commitSearch(value: string) {
    if (searchDebounce) clearTimeout(searchDebounce);""",
)

replace(
    "frontend/src/lib/utils/tagSuggestions.ts",
    "  const colon = query.indexOf(':');",
    """  // The base key count is the unique-file aggregate for both the plain key and
  // its namespace completion. Promote derived namespace counts to that same
  // aggregate when the base candidate is present; a plain-only key still never
  // creates a namespace candidate.
  for (const [namespaceName, item] of seenNamespaces) {
    const base = seenTags.get(namespaceName.slice(0, -1));
    if (base) seenNamespaces.set(namespaceName, { ...item, count: base.count });
  }

  const colon = query.indexOf(':');""",
)

replace(
    "internal/database/database.go",
    """\trows, err := s.Query(`
\t\tSELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
\t\tFROM tags
\t\tWHERE lower(key) LIKE ? OR lower(key || ':' || value) LIKE ?
\t\tORDER BY files_count DESC, tag_str ASC
\t\tLIMIT ?
\t`, like, like, limit)""",
    """\trows, err := s.Query(`
\t\tWITH matching_keys AS (
\t\t\tSELECT t.key AS tag_str, COUNT(DISTINCT ct.content_hash) AS files_count
\t\t\tFROM tags t
\t\t\tJOIN content_tags ct ON ct.tag_id = t.id
\t\t\tWHERE lower(t.key) LIKE ?
\t\t\tGROUP BY t.key
\t\t), matching_values AS (
\t\t\tSELECT key || ':' || value AS tag_str, files_count
\t\t\tFROM tags
\t\t\tWHERE value != '' AND lower(key || ':' || value) LIKE ?
\t\t)
\t\tSELECT tag_str, files_count FROM matching_keys
\t\tUNION ALL
\t\tSELECT tag_str, files_count FROM matching_values
\t\tORDER BY files_count DESC, tag_str ASC
\t\tLIMIT ?
\t`, like, like, limit)""",
)
replace(
    "internal/database/database.go",
    """\trows, err := s.Query(`
\t\tSELECT key || ':' AS tag_str, SUM(files_count) AS total_files
\t\tFROM tags
\t\tWHERE value != '' AND lower(key) LIKE ?
\t\tGROUP BY key
\t\tORDER BY total_files DESC, tag_str ASC
\t\tLIMIT ?
\t`, like, limit)""",
    """\trows, err := s.Query(`
\t\tSELECT t.key || ':' AS tag_str, COUNT(DISTINCT ct.content_hash) AS total_files
\t\tFROM tags t
\t\tJOIN content_tags ct ON ct.tag_id = t.id
\t\tWHERE t.value != '' AND lower(t.key) LIKE ?
\t\tGROUP BY t.key
\t\tORDER BY total_files DESC, tag_str ASC
\t\tLIMIT ?
\t`, like, limit)""",
)

p = Path("frontend/src/lib/utils/tagSuggestions.test.ts")
text = p.read_text()
marker = "describe('plain tag suggestions', () => {"
if marker not in text:
    raise SystemExit("tag suggestion test describe missing")
addition = """
  it('uses the base unique-file count for a real namespace and never invents namespaces for plain tags', () => {
    const candidates = [
      { name: 'animal', count: 3 },
      { name: 'animal:cat', namespace: 'animal', value: 'cat', count: 1 },
      { name: 'animal:hamster', namespace: 'animal', value: 'hamster', count: 1 },
      { name: 'animal:horse', namespace: 'animal', value: 'horse', count: 1 },
      { name: 'ai', count: 2 },
      { name: 'a', count: 1 }
    ];

    const suggestions = plainTagSuggestions('a', candidates, [], 10);
    expect(suggestions.slice(0, 4)).toEqual([
      { name: 'animal', count: 3, kind: 'tag' },
      { name: 'animal:', count: 3, kind: 'namespace' },
      { name: 'ai', count: 2, kind: 'tag' },
      { name: 'a', count: 1, kind: 'tag' }
    ]);
    expect(suggestions.some(({ name }) => name === 'a:' || name === 'ai:')).toBe(false);
  });
"""
p.write_text(text.replace(marker, marker + addition, 1))

p = Path("frontend/src/lib/state/libraryWorkflow.test.ts")
p.write_text(
    p.read_text()
    + """

describe('completion responsiveness', () => {
  it('publishes suggestion drafts immediately while keeping library submission debounced', () => {
    vi.useFakeTimers();
    try {
      const library = createLibraryWorkflow();
      library.setSearchDraft('animal');
      expect(storeValue(library.suggestionSearch)).toBe('animal');

      library.setSearch('animal:cat');
      expect(storeValue(library.suggestionSearch)).toBe('animal:cat');
      expect(storeValue(library.submittedSearch)).toBe('');
      vi.advanceTimersByTime(280);
      expect(storeValue(library.submittedSearch)).toBe('animal:cat');
    } finally {
      vi.useRealTimers();
    }
  });
});
"""
)

p = Path("frontend/tests/search-completion-ranking.spec.ts")
p.write_text(
    p.read_text()
    + """

test('rapid completion typing keeps the popup stable and request responses bound to their exact query', async ({ page }) => {
  await mockApp(page);
  const seenQueries: string[] = [];
  await page.unroute('**/api/v1/search/suggestions?**');
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const q = new URL(route.request().url()).searchParams.get('q') ?? '';
    seenQueries.push(q);
    await new Promise((resolve) => setTimeout(resolve, 75));
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [],
        meta_tags: [{ syntax: '@filename_contains:', hint: 'filename contains', requires_value: true }]
      })
    });
  });

  const search = page.getByLabel('Search library');
  await search.fill('@f');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  for (const suffix of ['i', 'l', 'e', 'n', 'a', 'm', 'e']) {
    await search.press(suffix);
    await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
    await expect(page.locator('#searchbar-suggestions [role=\"option\"]').first()).toContainText('@filename_contains');
  }
  await expect.poll(() => seenQueries.at(-1)).toBe('@filename');
  expect(seenQueries.every((query) => '@filename'.startsWith(query))).toBe(true);
});
"""
)

p = Path("internal/database/database_test.go")
p.write_text(
    p.read_text()
    + r'''

func TestTagSuggestionsUseUniqueFileAggregatesForBaseAndNamespace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := CreateEmptyDB(dbPath); err != nil {
		t.Fatalf("CreateEmptyDB: %v", err)
	}
	store, err := NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES ('h1'), ('h2'), ('h3')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('animal',''),('animal','cat'),('animal','hamster'),('animal','horse'),('ai',''),('a','')`); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][3]string{
		{"h1", "animal", ""}, {"h1", "animal", "cat"}, {"h1", "animal", "horse"},
		{"h2", "animal", "hamster"}, {"h3", "animal", "horse"},
		{"h1", "ai", ""}, {"h2", "ai", ""}, {"h3", "a", ""},
	} {
		if _, err := store.Exec(`INSERT INTO content_tags (content_hash, tag_id) SELECT ?, id FROM tags WHERE key = ? AND value = ?`, pair[0], pair[1], pair[2]); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := store.ListTagSuggestions("a", 20)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, tag := range tags {
		counts[tag.Tag] = tag.Count
	}
	if counts["animal"] != 3 {
		t.Fatalf("animal count=%d want 3: %#v", counts["animal"], tags)
	}
	if counts["ai"] != 2 || counts["a"] != 1 {
		t.Fatalf("plain counts wrong: %#v", tags)
	}

	namespaces, err := store.ListNamespaceSuggestions("a", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(namespaces) != 1 || namespaces[0].Tag != "animal:" || namespaces[0].Count != 3 {
		t.Fatalf("namespace suggestions=%#v want animal: count 3", namespaces)
	}
}
'''
)
