package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDraftRequiresTitleAndBody(t *testing.T) {
	_, err := parseDraft("plain text only")
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title validation error, got %v", err)
	}

	_, err = parseDraft("# Title\n\n")
	if err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestParseDraftExtractsPrompt(t *testing.T) {
	prompt, err := parseDraft("# My Prompt\n\nBody line")
	if err != nil {
		t.Fatalf("parseDraft returned error: %v", err)
	}
	if prompt.Title != "My Prompt" {
		t.Fatalf("unexpected title %q", prompt.Title)
	}
	if !strings.Contains(prompt.Markdown, "Body line") {
		t.Fatalf("expected body in markdown, got %q", prompt.Markdown)
	}
}

func TestParsePromptFields(t *testing.T) {
	prompt, err := parsePromptFields("My Title", "Body text")
	if err != nil {
		t.Fatalf("parsePromptFields returned error: %v", err)
	}
	if prompt.Title != "My Title" {
		t.Fatalf("unexpected title %q", prompt.Title)
	}
	if !strings.Contains(prompt.Markdown, "# My Title") || !strings.Contains(prompt.Markdown, "Body text") {
		t.Fatalf("unexpected markdown %q", prompt.Markdown)
	}
}

func TestFormatAndReadPromptFile(t *testing.T) {
	prompt := Prompt{
		Title:     "Hello",
		Timestamp: time.Date(2026, 5, 8, 12, 30, 0, 0, time.UTC),
		Markdown:  "# Hello\n\nBody.\n",
	}

	formatted := formatPromptFile(prompt)
	if !strings.Contains(formatted, "timestamp: 2026-05-08T12:30:00Z") {
		t.Fatalf("missing timestamp in frontmatter: %q", formatted)
	}
}

func TestBuildPagePaginates(t *testing.T) {
	prompts := make([]Prompt, 51)
	for i := range prompts {
		prompts[i] = Prompt{
			Title:     "Prompt",
			Timestamp: time.Date(2026, 5, 8, 12, i, 0, 0, time.UTC),
			Markdown:  "# Prompt\n\nBody\n",
		}
	}

	page, err := buildPage(prompts, map[int]bool{}, 2)
	if err != nil {
		t.Fatalf("buildPage returned error: %v", err)
	}
	if len(page.Prompts) != 1 {
		t.Fatalf("expected 1 prompt on second page, got %d", len(page.Prompts))
	}
	if page.TotalPages != 2 {
		t.Fatalf("expected 2 pages, got %d", page.TotalPages)
	}
	if page.Prompts[0].Index != 0 {
		t.Fatalf("expected newest prompt on page 2 to be index 0, got %d", page.Prompts[0].Index)
	}
}

func TestCompilePromptsAll(t *testing.T) {
	prompts := []Prompt{
		{
			Title:     "First",
			Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
			Markdown:  "# First\n\nAlpha\n",
		},
		{
			Title:     "Second",
			Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC),
			Markdown:  "# Second\n\nBeta\n",
		},
	}

	compiled, err := compilePrompts(prompts, nil, "")
	if err != nil {
		t.Fatalf("compilePrompts returned error: %v", err)
	}
	if !strings.Contains(compiled, "<!-- prompt 0 | 2026-05-08T12:00:00Z -->") {
		t.Fatalf("expected first prompt metadata, got %q", compiled)
	}
	if !strings.Contains(compiled, "# Second\n\nBeta") {
		t.Fatalf("expected second prompt body, got %q", compiled)
	}
}

func TestCompilePromptsRange(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
		{Title: "Two", Timestamp: time.Date(2026, 5, 8, 12, 2, 0, 0, time.UTC), Markdown: "# Two\n\nC\n"},
	}

	compiled, err := compilePrompts(prompts, &compileRange{Start: 1, End: 2}, "")
	if err != nil {
		t.Fatalf("compilePrompts returned error: %v", err)
	}
	if strings.Contains(compiled, "# Zero") {
		t.Fatalf("did not expect prompt zero in compiled output: %q", compiled)
	}
	if !strings.Contains(compiled, "# One") || !strings.Contains(compiled, "# Two") {
		t.Fatalf("expected selected prompts in compiled output: %q", compiled)
	}
}

func TestCompilePromptsRejectsInvalidRange(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
	}

	_, err := compilePrompts(prompts, &compileRange{Start: 1, End: 1}, "")
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out of bounds error, got %v", err)
	}
}

func TestCompilePromptIndexesSortsSelection(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
		{Title: "Two", Timestamp: time.Date(2026, 5, 8, 12, 2, 0, 0, time.UTC), Markdown: "# Two\n\nC\n"},
	}

	compiled, err := compilePromptIndexes(prompts, []int{2, 0}, "")
	if err != nil {
		t.Fatalf("compilePromptIndexes returned error: %v", err)
	}
	first := strings.Index(compiled, "# Zero")
	second := strings.Index(compiled, "# Two")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("expected prompts to compile in chronological index order, got %q", compiled)
	}
}

func TestParseCompileArgs(t *testing.T) {
	target, err := parseCompileArgs([]string{"0", "2", "./out.txt"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if target.Range == nil || target.Range.Start != 0 || target.Range.End != 2 {
		t.Fatalf("unexpected compile range: %#v", target.Range)
	}
	if target.OutputFile != "./out.txt" {
		t.Fatalf("unexpected output file %q", target.OutputFile)
	}

	target, err = parseCompileArgs([]string{"./out.txt"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if target.OutputFile != "./out.txt" || target.Range != nil {
		t.Fatalf("unexpected target %#v", target)
	}
}

func TestCompilePromptIndexesIncludesPrefix(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
	}

	compiled, err := compilePromptIndexes(prompts, []int{0}, "# Prefix\n\nIntro")
	if err != nil {
		t.Fatalf("compilePromptIndexes returned error: %v", err)
	}
	if !strings.HasPrefix(compiled, "# Prefix\n\nIntro\n\n<!-- prompt 0") {
		t.Fatalf("expected prefix at top, got %q", compiled)
	}
}

func TestParseCompileArgsRejectsInvalidShape(t *testing.T) {
	_, err := parseCompileArgs([]string{"0", "./out.txt"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestParseDeleteArgs(t *testing.T) {
	rng, err := parseDeleteArgs([]string{"2", "4"})
	if err != nil {
		t.Fatalf("parseDeleteArgs returned error: %v", err)
	}
	if rng.Start != 2 || rng.End != 4 {
		t.Fatalf("unexpected delete range %#v", rng)
	}

	rng, err = parseDeleteArgs([]string{"3"})
	if err != nil {
		t.Fatalf("parseDeleteArgs returned error: %v", err)
	}
	if rng.Start != 3 || rng.End != 3 {
		t.Fatalf("unexpected single delete range %#v", rng)
	}
}

func TestReindexMarksAfterDelete(t *testing.T) {
	marks := map[int]bool{1: true, 2: true, 5: true}
	updated := reindexMarksAfterDelete(marks, deleteRange{Start: 2, End: 3})

	if !updated[1] {
		t.Fatalf("expected mark 1 to remain")
	}
	if updated[2] {
		t.Fatalf("did not expect deleted mark to remain")
	}
	if !updated[3] {
		t.Fatalf("expected original mark 5 to shift to 3")
	}
}

func TestParseSelectedPromptIndexesDeduplicatesAndSorts(t *testing.T) {
	indexes, err := parseSelectedPromptIndexes([]string{"3", "1", "3"}, 4)
	if err != nil {
		t.Fatalf("parseSelectedPromptIndexes returned error: %v", err)
	}
	if len(indexes) != 2 || indexes[0] != 1 || indexes[1] != 3 {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestParseSelectedPromptIndexesRequiresSelection(t *testing.T) {
	_, err := parseSelectedPromptIndexes(nil, 4)
	if err == nil || !strings.Contains(err.Error(), "select at least one") {
		t.Fatalf("expected empty selection error, got %v", err)
	}
}

func TestBuildCompileOptionsDefaultToUnchecked(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC)},
	}

	options := buildCompileOptions(prompts, map[int]bool{}, nil)
	if len(options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(options))
	}
	if options[0].Checked || options[1].Checked {
		t.Fatalf("expected all options to be unchecked by default: %#v", options)
	}
}

func TestBuildPageMarksVisiblePrompt(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
	}

	page, err := buildPage(prompts, map[int]bool{1: true}, 1)
	if err != nil {
		t.Fatalf("buildPage returned error: %v", err)
	}
	if !page.Prompts[0].Marked {
		t.Fatalf("expected newest prompt to be marked")
	}
}

func TestCurrentMarkedIndexReturnsHighestIndex(t *testing.T) {
	index, ok := currentMarkedIndex(map[int]bool{2: true, 5: true})
	if !ok || index != 5 {
		t.Fatalf("expected highest marked index 5, got %d %v", index, ok)
	}
}

func TestIndexesForwardFromMark(t *testing.T) {
	indexes := indexesForwardFromMark(8, map[int]bool{3: true})
	if len(indexes) != 4 {
		t.Fatalf("expected 4 indexes, got %d", len(indexes))
	}
	if indexes[0] != 4 || indexes[len(indexes)-1] != 7 {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestIndexesForwardFromMarkWithoutMark(t *testing.T) {
	indexes := indexesForwardFromMark(8, map[int]bool{})
	if indexes != nil {
		t.Fatalf("expected nil indexes without mark, got %#v", indexes)
	}
}

func TestIndexesForwardFromMarkAtEnd(t *testing.T) {
	indexes := indexesForwardFromMark(5, map[int]bool{4: true})
	if len(indexes) != 0 {
		t.Fatalf("expected no indexes when mark is latest, got %#v", indexes)
	}
}

func TestRunInitCreatesProjectNote(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	notePath := filepath.Join(tempDir, projectNoteFileName)
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(note), "Prompt Memory Project") && !strings.Contains(string(note), "pmp") {
		t.Fatalf("expected project note contents, got %q", string(note))
	}
}

func TestMarkCompiledPromptReplacesExistingMarks(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if err := saveMarks(map[int]bool{1: true, 3: true}); err != nil {
		t.Fatalf("saveMarks returned error: %v", err)
	}
	if err := markCompiledPrompt([]int{2, 4}); err != nil {
		t.Fatalf("markCompiledPrompt returned error: %v", err)
	}

	marks, err := loadMarks()
	if err != nil {
		t.Fatalf("loadMarks returned error: %v", err)
	}
	if len(marks) != 1 || !marks[4] {
		t.Fatalf("expected only latest compiled prompt to remain marked, got %#v", marks)
	}
}

func TestEnsureProjectInitializedCreatesProject(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := ensureProjectInitialized(); err != nil {
		t.Fatalf("ensureProjectInitialized returned error: %v", err)
	}
	if err := ensureProject(); err != nil {
		t.Fatalf("expected initialized project, got %v", err)
	}
}

func TestLoadPrefixCreatesMissingFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	path, err := prefixPath()
	if err != nil {
		t.Fatalf("prefixPath returned error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	prefix, err := loadPrefix()
	if err != nil {
		t.Fatalf("loadPrefix returned error: %v", err)
	}
	if prefix != "" {
		t.Fatalf("expected empty prefix, got %q", prefix)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected recreated prefix file, got %v", err)
	}
}

func TestLoadAudioSettingsCreatesDefaultFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	path, err := audioSettingsPath()
	if err != nil {
		t.Fatalf("audioSettingsPath returned error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	settings, err := loadAudioSettings()
	if err != nil {
		t.Fatalf("loadAudioSettings returned error: %v", err)
	}
	if settings.WakeWord != "giraffe" || settings.SplitWord != "dash" || settings.SaveWord != "cucumber" {
		t.Fatalf("unexpected settings %#v", settings)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected recreated audio settings file, got %v", err)
	}
}

func TestWriteCompileJSONSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCompileJSON(rec, "hello", nil)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["compiled"] != "hello" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestWriteCompileJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCompileJSON(rec, "", os.ErrInvalid)

	if rec.Code != 400 {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["error"] == "" {
		t.Fatalf("expected error payload, got %#v", payload)
	}
}

func TestServeAddressIsConsistent(t *testing.T) {
	if serveAddress != "127.0.0.1:8765" {
		t.Fatalf("unexpected serve address %q", serveAddress)
	}
}
