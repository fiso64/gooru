from pathlib import Path

# Core persistence regression: reordered IDs survive a later metadata update without
# rewriting identity/query/title and newly-created searches append at the end.
p = Path('gooru/saved_search_test.go')
text = p.read_text()
if 'TestSavedSearchReorderPersistsWithoutRewritingSearches' not in text:
    text += r'''

func TestSavedSearchReorderPersistsWithoutRewritingSearches(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()
	if _, err := client.store.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`, "usr_test", "alice", "unused-test-hash", "admin"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	first, err := client.CreateSavedSearchForUsername("alice", "First", "kind:image", "name", "asc")
	if err != nil { t.Fatalf("create first: %v", err) }
	second, err := client.CreateSavedSearchForUsername("alice", "Second", "kind:video", "size", "desc")
	if err != nil { t.Fatalf("create second: %v", err) }
	third, err := client.CreateSavedSearchForUsername("alice", "Third", "", "added", "desc")
	if err != nil { t.Fatalf("create third: %v", err) }

	wantIDs := []string{third.ID, first.ID, second.ID}
	if err := client.ReorderSavedSearches("usr_test", wantIDs); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	items, err := client.ListSavedSearches("usr_test")
	if err != nil { t.Fatalf("list reordered: %v", err) }
	for i, wantID := range wantIDs {
		if items[i].ID != wantID || items[i].Position != i {
			t.Fatalf("item %d = %+v, want id=%s position=%d", i, items[i], wantID, i)
		}
	}
	if items[0].Name != "Third" || items[0].Query != "" || items[1].Name != "First" || items[1].Query != "kind:image" {
		t.Fatalf("reorder rewrote saved search content: %+v", items)
	}

	if _, err := client.UpdateSavedSearchForUsername("alice", first.ID, "First renamed", "kind:image", "name", "asc"); err != nil {
		t.Fatalf("update reordered search: %v", err)
	}
	items, err = client.ListSavedSearches("usr_test")
	if err != nil { t.Fatalf("list after update: %v", err) }
	for i, wantID := range wantIDs {
		if items[i].ID != wantID {
			t.Fatalf("metadata update changed order: got=%v want=%v", []string{items[0].ID, items[1].ID, items[2].ID}, wantIDs)
		}
	}
	if items[1].Name != "First renamed" || items[1].Query != "kind:image" {
		t.Fatalf("metadata update corrupted saved search: %+v", items[1])
	}

	fourth, err := client.CreateSavedSearchForUsername("alice", "Fourth", "", "name", "asc")
	if err != nil { t.Fatalf("create fourth: %v", err) }
	items, err = client.ListSavedSearches("usr_test")
	if err != nil { t.Fatalf("list after append: %v", err) }
	if got := items[len(items)-1].ID; got != fourth.ID {
		t.Fatalf("new saved search id=%s not appended; last=%s", fourth.ID, got)
	}

	if err := client.ReorderSavedSearches("usr_test", []string{first.ID, second.ID}); err == nil {
		t.Fatal("expected incomplete reorder to be rejected")
	}
	if err := client.ReorderSavedSearches("usr_test", []string{first.ID, second.ID, third.ID, third.ID}); err == nil {
		t.Fatal("expected duplicate reorder to be rejected")
	}
}
'''
p.write_text(text)

# Browser/API boundary regression: exact order payload, PUT method, and CSRF.
p = Path('frontend/src/lib/api/client.test.ts')
text = p.read_text()
anchor = "  it('maps API error envelopes', async () => {"
insert = r'''  it('persists saved-search order through the dedicated reorder endpoint', async () => {
    const requests: Array<{ url: string; method: string; headers: Headers; body: string }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        method: request.method,
        headers: request.headers,
        body: await request.clone().text()
      });
      return Response.json({ ok: true });
    }) as typeof fetch;

    await new ApiClient('secret-token').reorderSavedSearches(['saved-three', 'saved-one', 'saved-two']);

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/api/v1/saved-searches/reorder');
    expect(requests[0].method).toBe('PUT');
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(JSON.parse(requests[0].body)).toEqual({ ids: ['saved-three', 'saved-one', 'saved-two'] });
  });

'''
if insert not in text:
    if anchor not in text:
        raise SystemExit('client test anchor missing')
    text = text.replace(anchor, insert + anchor, 1)
p.write_text(text)
