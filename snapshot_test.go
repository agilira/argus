// snapshot_test.go - tests for the versioned snapshot and the validated swap
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// docSnapshot builds a candidate holding one key and one document that agree
// with each other, so that a reader catching a torn state can see it: the key
// and the document always carry the same generation number.
func docSnapshot(t *testing.T, generation int) *snapshot {
	t.Helper()
	text := fmt.Sprintf("generation %d", generation)
	return &snapshot{
		file: map[string]interface{}{"generation": generation},
		docs: map[DocumentID]Document{
			{Group: "prompts", Name: "system"}: newDocument("prompts", "system", "/tmp/system.md", text, time.Unix(int64(generation), 0)),
		},
	}
}

// TestSnapshot_RevisionIsMonotonic: every accepted candidate gets the next
// number, and nothing else does.
func TestSnapshot_RevisionIsMonotonic(t *testing.T) {
	r := newResolver()

	if got := r.revision(); got != 0 {
		t.Errorf("revision before the first load = %d, want 0", got)
	}

	for want := uint64(1); want <= 5; want++ {
		got, err := r.apply(docSnapshot(t, int(want)), nil)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if got != want {
			t.Errorf("apply returned revision %d, want %d", got, want)
		}
		if live := r.revision(); live != want {
			t.Errorf("revision() = %d, want %d", live, want)
		}
	}
}

// TestSnapshot_RefusedCandidateLeavesThePreviousOne: a candidate that does not
// validate is dropped whole. The application keeps serving the last good
// revision rather than coming up on half a configuration.
func TestSnapshot_RefusedCandidateLeavesThePreviousOne(t *testing.T) {
	r := newResolver()
	if _, err := r.apply(docSnapshot(t, 1), nil); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	refused := errors.New("required document missing")
	rev, err := r.apply(docSnapshot(t, 2), func(*snapshot) error { return refused })

	if !errors.Is(err, refused) {
		t.Errorf("apply error = %v, want %v", err, refused)
	}
	if rev != 1 {
		t.Errorf("apply returned revision %d, want the unchanged 1", rev)
	}
	if got := r.resolve("generation"); got.value != 1 {
		t.Errorf("generation = %v, want the previous revision's 1", got.value)
	}
	doc, ok := r.document("prompts", "system")
	if !ok || doc.String() != "generation 1" {
		t.Errorf("document = %q, want the previous revision's content", doc.String())
	}
}

// TestSnapshot_DocumentRevisionStamping: a document carries the revision its
// content arrived in, not the revision that happened to be current. A prompt
// that did not change does not look changed.
func TestSnapshot_DocumentRevisionStamping(t *testing.T) {
	r := newResolver()
	if _, err := r.apply(docSnapshot(t, 1), nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Same content, new candidate: the document keeps revision 1.
	unchanged := docSnapshot(t, 1)
	unchanged.file = map[string]interface{}{"generation": 2}
	if _, err := r.apply(unchanged, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	doc, _ := r.document("prompts", "system")
	if doc.Revision != 1 {
		t.Errorf("unchanged document Revision = %d, want 1", doc.Revision)
	}

	// New content: the document is stamped with the revision that carried it.
	if _, err := r.apply(docSnapshot(t, 3), nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	doc, _ = r.document("prompts", "system")
	if doc.Revision != 3 {
		t.Errorf("changed document Revision = %d, want 3", doc.Revision)
	}
	if doc.Hash == "" {
		t.Error("document has no hash; the audit trail records hashes, not content")
	}
}

// TestSnapshot_ReadersNeverSeeATornState hammers the readers while candidates
// are swapped in underneath them. A reader must see one revision whole: the
// key and the document it reads always come from the same one. Run with -race.
func TestSnapshot_ReadersNeverSeeATornState(t *testing.T) {
	r := newResolver()
	if _, err := r.apply(docSnapshot(t, 0), nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	torn := make(chan string, 1)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				view := r.view()
				generation, _ := view.file["generation"].(int)
				doc := view.docs[DocumentID{Group: "prompts", Name: "system"}]
				if doc.String() != "generation "+strconv.Itoa(generation) {
					select {
					case torn <- fmt.Sprintf("key says %d, document says %q", generation, doc.String()):
					default:
					}
					return
				}
			}
		}()
	}

	for generation := 1; generation <= 2000; generation++ {
		if _, err := r.apply(docSnapshot(t, generation), nil); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	close(stop)
	wg.Wait()

	select {
	case msg := <-torn:
		t.Fatalf("torn read: %s", msg)
	default:
	}
}

// TestSnapshot_ConcurrentApply: two reloaders racing to publish must still
// produce distinct, increasing revisions. One of them wins each number.
func TestSnapshot_ConcurrentApply(t *testing.T) {
	r := newResolver()

	const applies = 200
	seen := make([]uint64, applies)

	var wg sync.WaitGroup
	for i := 0; i < applies; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rev, err := r.apply(docSnapshot(t, i), nil)
			if err != nil {
				t.Errorf("apply: %v", err)
				return
			}
			seen[i] = rev
		}(i)
	}
	wg.Wait()

	if got := r.revision(); got != applies {
		t.Errorf("revision after %d applies = %d", applies, got)
	}
	taken := make(map[uint64]bool, applies)
	for _, rev := range seen {
		if taken[rev] {
			t.Fatalf("revision %d handed out twice", rev)
		}
		taken[rev] = true
	}
}

// TestChange_Helpers: what OnReload hands the application is a list of names,
// not a diff of values. A prompt's new text must not travel through a callback
// argument that ends up in a log line.
func TestChange_Helpers(t *testing.T) {
	change := Change{
		Revision:  7,
		Keys:      []string{"model", "temperature"},
		Documents: []DocumentID{{Group: "prompts", Name: "system"}},
	}

	if !change.HasKey("model") || change.HasKey("missing") {
		t.Error("HasKey is wrong")
	}
	if !change.HasDocument("prompts", "system") {
		t.Error("HasDocument(prompts, system) = false")
	}
	if change.HasDocument("skills", "system") || change.HasDocument("prompts", "other") {
		t.Error("HasDocument matched the wrong group or name")
	}
}

// TestDocument_BytesAreACopy: a snapshot is shared by every reader, so handing
// out the backing array would let one caller rewrite another's prompt.
func TestDocument_BytesAreACopy(t *testing.T) {
	doc := newDocument("prompts", "system", "/tmp/system.md", "be careful", time.Unix(0, 0))

	first := doc.Bytes()
	first[0] = 'X'

	if doc.String() != "be careful" {
		t.Errorf("document content changed to %q through the returned slice", doc.String())
	}
	if second := doc.Bytes(); string(second) != "be careful" {
		t.Errorf("second Bytes() = %q", second)
	}
}

// TestSnapshot_DeriveDoesNotWriteIntoThePublishedRevision: replacing one layer
// derives a candidate from the current snapshot, which means the two share
// their document map. Stamping the candidate must not write through that
// sharing into a revision other goroutines are reading. Run with -race.
func TestSnapshot_DeriveDoesNotWriteIntoThePublishedRevision(t *testing.T) {
	r := newResolver()
	if _, err := r.apply(docSnapshot(t, 1), nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if doc, ok := r.document("prompts", "system"); ok && doc.String() == "" {
						t.Error("document lost its content mid-swap")
						return
					}
				}
			}
		}()
	}

	for i := 0; i < 500; i++ {
		if err := r.setFile(map[string]interface{}{"generation": i}); err != nil {
			t.Fatalf("setFile: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}
