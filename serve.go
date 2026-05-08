package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pageSize = 50
const serveAddress = "127.0.0.1:8765"

type navItem struct {
	Label   string
	Href    string
	Current bool
}

type newPromptPageData struct {
	Nav           []navItem
	Error         string
	Title         string
	Body          string
	Saved         bool
	AudioSettings AudioSettings
}

type prefixPageData struct {
	Nav    []navItem
	Error  string
	Saved  bool
	Prefix string
}

type settingsPageData struct {
	Nav           []navItem
	Error         string
	Saved         bool
	AudioSettings AudioSettings
}

type promptListPage struct {
	Nav          []navItem
	Prompts      []PromptView
	Page         int
	TotalPages   int
	PrevPage     int
	NextPage     int
	TotalPrompts int
}

type PromptView struct {
	Index     int
	Title     string
	Timestamp string
	HTMLBody  template.HTML
	ElementID string
	Marked    bool
}

type compilePageData struct {
	Nav          []navItem
	Options      []CompileOption
	Error        string
	Copied       bool
	MarkedIndex  int
	HasMark      bool
	TotalPrompts int
}

type CompileOption struct {
	Index     int
	Title     string
	Timestamp string
	Checked   bool
	Marked    bool
}

type websocketHub struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

type projectSnapshot struct {
	FileCount int
	LatestMod int64
}

func newWebsocketHub() *websocketHub {
	return &websocketHub{conns: map[net.Conn]struct{}{}}
}

func (h *websocketHub) add(conn net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn] = struct{}{}
}

func (h *websocketHub) remove(conn net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, conn)
}

func (h *websocketHub) broadcastText(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.conns {
		if err := writeWebsocketTextFrame(conn, message); err != nil {
			_ = conn.Close()
			delete(h.conns, conn)
		}
	}
}

func runServe() error {
	if err := ensureProject(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", serveAddress)
	if err != nil {
		return err
	}

	hub := newWebsocketHub()
	go watchProjectChanges(hub)

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveHome)
	mux.HandleFunc("/new", serveNewPrompt)
	mux.HandleFunc("/prefix", servePrefix)
	mux.HandleFunc("/settings", serveSettings)
	mux.HandleFunc("/prompts", servePrompts)
	mux.HandleFunc("/prompts/delete", serveDeletePrompt)
	mux.HandleFunc("/compile", serveCompile)
	mux.HandleFunc("/ws", hub.handle)

	url := "http://" + listener.Addr().String()
	_ = openBrowser(url)
	fmt.Println(url)
	return http.Serve(listener, mux)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/new", http.StatusSeeOther)
}

func servePrompts(w http.ResponseWriter, r *http.Request) {
	prompts, marks, err := loadPromptState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			http.Error(w, "invalid page", http.StatusBadRequest)
			return
		}
		page = n
	}

	data, err := buildPage(prompts, marks, page)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errPageOutOfRange) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	data.Nav = buildNav("/prompts")
	data.TotalPrompts = len(prompts)

	renderTemplate(w, promptsTemplate, data)
}

func serveCompile(w http.ResponseWriter, r *http.Request) {
	prompts, marks, err := loadPromptState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := compilePageData{
		Nav:          buildNav("/compile"),
		Options:      buildCompileOptions(prompts, marks, nil),
		TotalPrompts: len(prompts),
	}
	data.MarkedIndex, data.HasMark = currentMarkedIndex(marks)
	if r.URL.Query().Get("copied") == "1" {
		data.Copied = true
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			writeCompileJSON(w, "", err)
			return
		}

		selected, err := parseSelectedPromptIndexes(r.Form["prompt"], len(prompts))
		data.Options = buildCompileOptions(prompts, marks, selected)
		if err != nil {
			writeCompileJSON(w, "", err)
			return
		} else {
			prefix, err := loadPrefix()
			if err != nil {
				writeCompileJSON(w, "", err)
				return
			}
			compiled, err := compilePromptIndexes(prompts, selected, prefix)
			if err != nil {
				writeCompileJSON(w, "", err)
				return
			} else {
				if err := markCompiledPrompt(selected); err != nil {
					writeCompileJSON(w, "", err)
					return
				} else {
					marks = map[int]bool{selected[len(selected)-1]: true}
					data.Options = buildCompileOptions(prompts, marks, selected)
					data.Copied = true
					writeCompileJSON(w, compiled, nil)
					return
				}
			}
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	renderTemplate(w, compileTemplate, data)
}

func serveNewPrompt(w http.ResponseWriter, r *http.Request) {
	audioSettings, err := loadAudioSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newPromptPageData{
		Nav:           buildNav("/new"),
		AudioSettings: audioSettings,
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, newPromptTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		data.Title = r.Form.Get("title")
		data.Body = r.Form.Get("body")
		prompt, err := parsePromptFields(data.Title, data.Body)
		if err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		if _, err := savePrompt(prompt); err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		http.Redirect(w, r, "/new?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveSettings(w http.ResponseWriter, r *http.Request) {
	audioSettings, err := loadAudioSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := settingsPageData{
		Nav:           buildNav("/settings"),
		AudioSettings: audioSettings,
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, settingsTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, settingsTemplate, data)
			return
		}
		data.AudioSettings = normalizeAudioSettings(AudioSettings{
			WakeWord:  r.Form.Get("wake_word"),
			SplitWord: r.Form.Get("split_word"),
			SaveWord:  r.Form.Get("save_word"),
		})
		if err := saveAudioSettings(data.AudioSettings); err != nil {
			data.Error = err.Error()
			renderTemplate(w, settingsTemplate, data)
			return
		}
		http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func servePrefix(w http.ResponseWriter, r *http.Request) {
	data := prefixPageData{
		Nav: buildNav("/prefix"),
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	prefix, err := loadPrefix()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Prefix = prefix

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, prefixTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, prefixTemplate, data)
			return
		}
		data.Prefix = r.Form.Get("prefix")
		if err := savePrefix(data.Prefix); err != nil {
			data.Error = err.Error()
			renderTemplate(w, prefixTemplate, data)
			return
		}
		http.Redirect(w, r, "/prefix?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveDeletePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(r.Form.Get("index"))
	if err != nil {
		http.Error(w, "invalid prompt index", http.StatusBadRequest)
		return
	}
	if err := runDelete(deleteRange{Start: index, End: index}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/prompts", http.StatusSeeOther)
}

func loadPromptState() ([]Prompt, map[int]bool, error) {
	prompts, err := loadPrompts()
	if err != nil {
		return nil, nil, err
	}
	marks, err := loadMarks()
	if err != nil {
		return nil, nil, err
	}
	return prompts, marks, nil
}

func currentMarkedIndex(marks map[int]bool) (int, bool) {
	if len(marks) == 0 {
		return 0, false
	}

	indexes := make([]int, 0, len(marks))
	for index := range marks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes[len(indexes)-1], true
}

func markCompiledPrompt(selected []int) error {
	if len(selected) == 0 {
		return errors.New("select at least one prompt to compile")
	}

	return saveMarks(map[int]bool{selected[len(selected)-1]: true})
}

func buildNav(current string) []navItem {
	return []navItem{
		{Label: "New", Href: "/new", Current: current == "/new"},
		{Label: "Settings", Href: "/settings", Current: current == "/settings"},
		{Label: "Prefix", Href: "/prefix", Current: current == "/prefix"},
		{Label: "Prompts", Href: "/prompts", Current: current == "/prompts"},
		{Label: "Compile", Href: "/compile", Current: current == "/compile"},
	}
}

func renderTemplate(w http.ResponseWriter, tpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeCompileJSON(w http.ResponseWriter, compiled string, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	payload := map[string]string{}
	if err != nil {
		payload["error"] = err.Error()
		w.WriteHeader(http.StatusBadRequest)
	} else {
		payload["compiled"] = compiled
	}
	_ = json.NewEncoder(w).Encode(payload)
}

var errPageOutOfRange = errors.New("page out of range")

func buildPage(prompts []Prompt, marks map[int]bool, page int) (promptListPage, error) {
	totalPages := 1
	if len(prompts) > 0 {
		totalPages = (len(prompts) + pageSize - 1) / pageSize
	}
	if page > totalPages {
		return promptListPage{}, errPageOutOfRange
	}

	startOffset := (page - 1) * pageSize
	endOffset := startOffset + pageSize
	if startOffset > len(prompts) {
		startOffset = len(prompts)
	}
	if endOffset > len(prompts) {
		endOffset = len(prompts)
	}

	views := make([]PromptView, 0, endOffset-startOffset)
	for offset := startOffset; offset < endOffset; offset++ {
		index := len(prompts) - 1 - offset
		prompt := prompts[index]
		views = append(views, PromptView{
			Index:     index,
			Title:     prompt.Title,
			Timestamp: prompt.Timestamp.Local().Format("2006-01-02 15:04:05 MST"),
			HTMLBody:  template.HTML(renderMarkdown(prompt.Markdown)),
			ElementID: fmt.Sprintf("prompt-%d", index),
			Marked:    marks[index],
		})
	}

	data := promptListPage{
		Prompts:    views,
		Page:       page,
		TotalPages: totalPages,
	}
	if page > 1 {
		data.PrevPage = page - 1
	}
	if page < totalPages {
		data.NextPage = page + 1
	}
	return data, nil
}

func buildCompileOptions(prompts []Prompt, marks map[int]bool, selected []int) []CompileOption {
	selectedSet := map[int]bool{}
	for _, index := range selected {
		selectedSet[index] = true
	}

	options := make([]CompileOption, 0, len(prompts))
	for i := len(prompts) - 1; i >= 0; i-- {
		prompt := prompts[i]
		options = append(options, CompileOption{
			Index:     i,
			Title:     prompt.Title,
			Timestamp: prompt.Timestamp.Local().Format("2006-01-02 15:04:05 MST"),
			Checked:   selectedSet[i],
			Marked:    marks[i],
		})
	}
	return options
}

func indexesForwardFromMark(promptCount int, marks map[int]bool) []int {
	markedIndex, ok := currentMarkedIndex(marks)
	if !ok {
		return nil
	}

	if markedIndex+1 >= promptCount {
		return []int{}
	}

	indexes := make([]int, 0, promptCount-markedIndex-1)
	for i := markedIndex + 1; i < promptCount; i++ {
		indexes = append(indexes, i)
	}
	return indexes
}

func parseSelectedPromptIndexes(values []string, promptCount int) ([]int, error) {
	if len(values) == 0 {
		return nil, errors.New("select at least one prompt to compile")
	}

	selected := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		index, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid prompt index %q", value)
		}
		if index < 0 || index >= promptCount {
			return nil, fmt.Errorf("prompt index %d is out of bounds; highest prompt index is %d", index, promptCount-1)
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		selected = append(selected, index)
	}

	sort.Ints(selected)
	return selected, nil
}

func renderMarkdown(markdown string) string {
	var buf bytes.Buffer
	lines := splitLines(markdown)
	inParagraph := false
	inPre := false

	closeParagraph := func() {
		if inParagraph {
			buf.WriteString("</p>")
			inParagraph = false
		}
	}

	for _, line := range lines {
		trimmed := line
		if trimmed == "```" {
			closeParagraph()
			if inPre {
				buf.WriteString("</code></pre>")
			} else {
				buf.WriteString("<pre><code>")
			}
			inPre = !inPre
			continue
		}
		if inPre {
			template.HTMLEscape(&buf, []byte(line))
			buf.WriteByte('\n')
			continue
		}
		if trimmed == "" {
			closeParagraph()
			continue
		}
		if len(trimmed) > 2 && trimmed[:2] == "# " {
			closeParagraph()
			buf.WriteString("<h1>")
			template.HTMLEscape(&buf, []byte(trimmed[2:]))
			buf.WriteString("</h1>")
			continue
		}
		if !inParagraph {
			buf.WriteString("<p>")
			inParagraph = true
		} else {
			buf.WriteByte(' ')
		}
		template.HTMLEscape(&buf, []byte(trimmed))
	}

	closeParagraph()
	if inPre {
		buf.WriteString("</code></pre>")
	}
	return buf.String()
}

func splitLines(s string) []string {
	lines := make([]string, 0)
	current := bytes.Buffer{}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			continue
		}
		if s[i] != '\r' {
			current.WriteByte(s[i])
		}
	}
	lines = append(lines, current.String())
	return lines
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func watchProjectChanges(hub *websocketHub) {
	previous, err := snapshotProject()
	if err != nil {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		current, err := snapshotProject()
		if err != nil {
			continue
		}
		if current != previous {
			previous = current
			hub.broadcastText("reload")
		}
	}
}

func snapshotProject() (projectSnapshot, error) {
	_, promptsDir, _, err := projectPaths()
	if err != nil {
		return projectSnapshot{}, err
	}

	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return projectSnapshot{}, err
	}

	snapshot := projectSnapshot{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return projectSnapshot{}, err
		}
		snapshot.FileCount++
		mod := info.ModTime().UnixNano()
		if mod > snapshot.LatestMod {
			snapshot.LatestMod = mod
		}
	}

	marks, err := marksPath()
	if err != nil {
		return projectSnapshot{}, err
	}
	if info, err := os.Stat(marks); err == nil {
		mod := info.ModTime().UnixNano()
		if mod > snapshot.LatestMod {
			snapshot.LatestMod = mod
		}
	}

	return snapshot, nil
}

func (h *websocketHub) handle(w http.ResponseWriter, r *http.Request) {
	if !headerContainsToken(r.Header, "Connection", "Upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}

	accept := websocketAccept(key)
	if _, err := rw.WriteString(
		"HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n",
	); err != nil {
		_ = conn.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	h.add(conn)
	go h.readLoop(conn)
}

func (h *websocketHub) readLoop(conn net.Conn) {
	defer func() {
		h.remove(conn)
		_ = conn.Close()
	}()

	buffer := make([]byte, 4096)
	for {
		if _, err := conn.Read(buffer); err != nil {
			return
		}
	}
}

func headerContainsToken(header http.Header, key string, token string) bool {
	for _, value := range header.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebsocketTextFrame(conn net.Conn, message string) error {
	payload := []byte(message)
	frame := []byte{0x81}

	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		return errors.New("websocket payload too large")
	}

	frame = append(frame, payload...)
	_, err := conn.Write(frame)
	return err
}

const baseStyles = `
  :root {
    color-scheme: dark;
    --bg: #000000;
    --panel: #090909;
    --panel-strong: #111111;
    --border: #2f2f2f;
    --text: #ffffff;
    --muted: #a6a6a6;
    --accent: #ffffff;
    --action: #8fd18a;
    --mark-bg: #1a1a1a;
    --mark-border: #ffffff;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    background: #000000;
    color: var(--text);
    font: 14px/1.45 Georgia, "Times New Roman", serif;
  }
  a {
    color: var(--text);
    text-decoration: none;
  }
  .shell {
    width: min(72rem, calc(100% - 1rem));
    margin: 0 auto;
    padding: 0.75rem 0 1rem;
  }
  .nav {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-bottom: 0.75rem;
  }
  .nav a, .button, button {
    border: 1px solid var(--border);
    background: var(--panel-strong);
    color: var(--text);
    border-radius: 0.55rem;
    padding: 0.45rem 0.7rem;
    font: inherit;
  }
  .nav a.current, .button.primary, button.primary {
    background: var(--action);
    color: #000000;
    border-color: var(--action);
  }
  .panel, details {
    border: 1px solid var(--border);
    background: var(--panel);
    border-radius: 0.8rem;
  }
  .panel {
    padding: 0.8rem 0.9rem;
  }
  .panel + .panel, .stack, .pager {
    margin-top: 0.75rem;
  }
  .compact {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .muted {
    color: var(--muted);
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .actions.spaced {
    margin-bottom: 0.8rem;
  }
  .icon-button {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }
  .icon-button svg {
    width: 0.95rem;
    height: 0.95rem;
    display: block;
    fill: currentColor;
  }
  .icon-button span {
    display: none;
  }
  .stack {
    display: grid;
    gap: 0.55rem;
  }
  details {
    padding: 0.7rem 0.85rem;
  }
  details.marked, .prompt-option.marked {
    border-color: var(--mark-border);
    background: var(--mark-bg);
  }
  summary {
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .summary-row {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    align-items: baseline;
  }
  .summary-title {
    font-size: 0.98rem;
  }
  .summary-meta, .small {
    color: var(--muted);
    font-size: 0.88rem;
  }
  .mark-badge {
    display: inline-block;
    margin-left: 0.45rem;
    padding: 0.05rem 0.35rem;
    border: 1px solid #ffffff;
    border-radius: 999px;
    font-size: 0.72rem;
    line-height: 1.3;
    vertical-align: middle;
  }
  article {
    margin-top: 0.7rem;
    padding-top: 0.7rem;
    border-top: 1px solid var(--border);
  }
  article p, article h1 {
    margin-top: 0;
  }
  .pager {
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .prompt-picker {
    display: grid;
    gap: 0.45rem;
  }
  .prompt-option {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.65rem;
    align-items: start;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 0.7rem;
    background: #050505;
  }
  .prompt-option input {
    margin-top: 0.15rem;
  }
  .error {
    color: #ffffff;
    border-color: #ffffff;
  }
  textarea, input[type="text"] {
    width: 100%;
    border: 1px solid var(--border);
    background: #050505;
    color: var(--text);
    border-radius: 0.7rem;
    padding: 0.7rem 0.8rem;
    font: inherit;
  }
  textarea {
    min-height: 18rem;
    resize: vertical;
  }
  .form-grid {
    display: grid;
    gap: 0.7rem;
  }
  .label {
    display: grid;
    gap: 0.35rem;
  }
  .label-row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }
  .dot {
    width: 0.65rem;
    height: 0.65rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: #050505;
    display: inline-block;
    transition: background-color 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
  }
  .dot.waiting {
    background: #e0b84d;
    border-color: #e0b84d;
    box-shadow: 0 0 0.45rem rgba(224, 184, 77, 0.65);
  }
  .dot.live {
    background: #d93025;
    border-color: #d93025;
    box-shadow: 0 0 0.45rem rgba(217, 48, 37, 0.7);
  }
  .dot.active {
    background: var(--action);
    border-color: var(--action);
    box-shadow: 0 0 0.45rem rgba(143, 209, 138, 0.6);
  }
  .dot.heard {
    transform: scale(1.18);
  }
  .mic-status {
    min-height: 1.2rem;
  }
  .instructions {
    color: var(--muted);
    font-size: 0.88rem;
    line-height: 1.35;
  }
  .prompt-actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 0.7rem;
  }
  .danger {
    border-color: #7a2f2f;
    color: #ffb8b8;
  }
  .clipboard-source {
    position: absolute;
    left: -9999px;
    width: 1px;
    height: 1px;
    opacity: 0;
  }
  @media (max-width: 640px) {
    body { font-size: 13px; }
    .summary-row, .compact, .pager {
      flex-direction: column;
      align-items: flex-start;
    }
  }
`

const liveReloadScript = `<script>
  (function() {
    var scheme = window.location.protocol === "https:" ? "wss://" : "ws://";
    var socket = new WebSocket(scheme + window.location.host + "/ws");
    socket.onmessage = function(event) {
      if (event.data === "reload") {
        window.location.reload();
      }
    };
  })();
</script>`

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    <section class="panel compact">
      <div>{{.TotalPrompts}} prompts{{if .HasMark}} <span class="muted">marked: {{.MarkedIndex}}</span>{{end}}</div>
      <div class="actions">
        <a class="button" href="/new">New</a>
        <a class="button" href="/prompts">Browse</a>
        <a class="button primary" href="/compile">Compile</a>
      </div>
    </section>
  </main>
  ` + liveReloadScript + `
</body>
</html>`))

var newPromptTemplate = template.Must(template.New("new-prompt").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp new</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <div class="actions spaced">
          <button type="button" class="icon-button" onclick="startMic()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15a3 3 0 0 0 3-3V7a3 3 0 0 0-6 0v5a3 3 0 0 0 3 3zm5-3a1 1 0 0 1 2 0 7 7 0 0 1-6 6.93V21h3a1 1 0 0 1 0 2H8a1 1 0 0 1 0-2h3v-2.07A7 7 0 0 1 5 12a1 1 0 0 1 2 0 5 5 0 0 0 10 0z"/></svg>
            <span>Mic</span>
          </button>
          <button type="button" class="icon-button" onclick="pauseMic()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7 5a1 1 0 0 1 1 1v12a1 1 0 1 1-2 0V6a1 1 0 0 1 1-1zm10 0a1 1 0 0 1 1 1v12a1 1 0 1 1-2 0V6a1 1 0 0 1 1-1z"/></svg>
            <span>Pause</span>
          </button>
          <button type="button" class="icon-button" onclick="clearTranscript()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3h6l1 2h4a1 1 0 1 1 0 2h-1l-1 12a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 7H4a1 1 0 1 1 0-2h4l1-2zm1.2 4L8 19h8L13.8 7h-3.6z"/></svg>
            <span>Clear text</span>
          </button>
        </div>
        <div class="compact">
          <div class="stack">
            <div id="mic-status" class="muted mic-status"></div>
            <div id="speech-debug" class="instructions" aria-live="polite"></div>
          </div>
          <span id="mic-live-dot" class="dot" aria-hidden="true"></span>
        </div>
        <label class="label">
          <span class="label-row"><span>Title</span><span id="title-dot" class="dot" aria-hidden="true"></span></span>
          <input id="prompt-title" type="text" name="title" value="{{.Title}}">
        </label>
        <label class="label">
          <span class="label-row"><span>Body</span><span id="body-dot" class="dot" aria-hidden="true"></span></span>
          <textarea id="prompt-body" name="body">{{.Body}}</textarea>
        </label>
        <div class="actions spaced">
          <button type="submit" class="primary">Save prompt</button>
        </div>
        <div class="instructions">
          Say {{.AudioSettings.WakeWord}} to start.<br>
          Say {{.AudioSettings.SplitWord}} to switch. Say {{.AudioSettings.SaveWord}} to save.
        </div>
      </form>
    </section>
  </main>
  <script>
    var recognition = null;
    var recognitionStarting = false;
    var recognitionActive = false;
    var shouldKeepListening = false;
    var micStream = null;
    var micPermissionDenied = false;
    var wakeArmed = false;
    var restartAfterWake = false;
    var isSubmitting = false;
    var currentField = 'title';
    var committedFields = { title: '', body: '' };
    var draftFields = { title: '', body: '' };
    var micStatus = document.getElementById('mic-status');
    var speechDebug = document.getElementById('speech-debug');
    var micLiveDot = document.getElementById('mic-live-dot');
    var titleDot = document.getElementById('title-dot');
    var bodyDot = document.getElementById('body-dot');
    var titleInput = document.getElementById('prompt-title');
    var bodyInput = document.getElementById('prompt-body');
    var audioSettings = {
      wakeWord: {{printf "%q" .AudioSettings.WakeWord}},
      splitWord: {{printf "%q" .AudioSettings.SplitWord}},
      saveWord: {{printf "%q" .AudioSettings.SaveWord}}
    };
    function playStartTone() {
      var AudioContextCtor = window.AudioContext || window.webkitAudioContext;
      if (!AudioContextCtor) {
        return;
      }
      try {
        var ctx = new AudioContextCtor();
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.type = 'sine';
        osc.frequency.value = 880;
        gain.gain.value = 0.03;
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.start();
        gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.12);
        osc.stop(ctx.currentTime + 0.12);
        osc.onended = function() {
          ctx.close();
        };
      } catch (err) {
      }
    }
    function setActiveField(field) {
      titleDot.classList.toggle('active', field === 'title');
      bodyDot.classList.toggle('active', field === 'body');
    }
    function setMicLive(state) {
      micLiveDot.classList.toggle('waiting', state === 'waiting');
      micLiveDot.classList.toggle('live', state === 'live');
    }
    function pulseHeard(dot) {
      dot.classList.add('heard');
      window.setTimeout(function() {
        dot.classList.remove('heard');
      }, 180);
    }
    function escapeRegExp(text) {
      return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }
    function spokenSplitToken() {
      return ' ' + audioSettings.splitWord + ' ';
    }
    function normalizeSpokenText(text) {
      return text
        .replace(/\s[-–—:]\s/g, spokenSplitToken())
        .replace(/[.,!?;()[\]{}"']/g, ' ')
        .replace(/\s+/g, ' ')
        .trim();
    }
    function commandRegex(command, flags) {
      var normalized = normalizeSpokenText(command);
      var escaped = escapeRegExp(normalized).replace(/\\ /g, '\\s+');
      return new RegExp('(^|\\s)(' + escaped + ')(?=\\s|$)', flags || 'i');
    }
    function wakeWordPattern() {
      return commandRegex(audioSettings.wakeWord, 'i');
    }
    function splitAfterWakeWord(text) {
      var match = wakeWordPattern().exec(text);
      if (!match) {
        return null;
      }
      return {
        after: normalizeSpokenText(text.slice(match.index + match[1].length + match[2].length))
      };
    }
    function splitAfterWakeWordFromAlternatives(result) {
      if (!result) {
        return null;
      }
      for (var j = 0; j < result.length; j++) {
        var transcript = normalizeSpokenText(result[j].transcript);
        if (!transcript) {
          continue;
        }
        var wakeSplit = splitAfterWakeWord(transcript);
        if (wakeSplit) {
          wakeSplit.transcript = transcript;
          return wakeSplit;
        }
      }
      return null;
    }
    function stripLeadingWakeWord(text) {
      return normalizeSpokenText(text.replace(commandRegex(audioSettings.wakeWord, 'i'), ' '));
    }
    function commandPattern() {
      var commands = [
        normalizeSpokenText(audioSettings.wakeWord),
        normalizeSpokenText(audioSettings.splitWord),
        normalizeSpokenText(audioSettings.saveWord)
      ].map(function(command) {
        return escapeRegExp(command).replace(/\\ /g, '\\s+');
      });
      return new RegExp('(^|\\s)(' + commands.join('|') + ')(?=\\s|$)', 'ig');
    }
    function renderPromptFields(title, body, field) {
      titleInput.value = title;
      bodyInput.value = body;
      setActiveField(field);
    }
    function fieldValue(field) {
      return [committedFields[field], draftFields[field]].join(' ').replace(/\s+/g, ' ').trim();
    }
    function renderFromState(field) {
      renderPromptFields(fieldValue('title'), fieldValue('body'), field);
    }
    function updateMicStatus() {
      if (!recognitionActive) {
        micStatus.textContent = shouldKeepListening ? 'connecting microphone' : 'paused';
        return;
      }
      if (!wakeArmed) {
        micStatus.textContent = 'listening for ' + audioSettings.wakeWord;
        return;
      }
      micStatus.textContent = currentField === 'body' ? 'capturing body' : 'capturing title';
    }
    function setSpeechDebug(message) {
      speechDebug.textContent = message || '';
    }
    function renderWakeListeningState() {
      setMicLive(recognitionActive ? 'waiting' : '');
      renderFromState('');
      updateMicStatus();
      setSpeechDebug('wake word: ' + audioSettings.wakeWord);
    }
    function renderCaptureState() {
      setMicLive('live');
      renderFromState(currentField);
      updateMicStatus();
      setSpeechDebug('wake word heard');
    }
    function resetDraftFields() {
      draftFields.title = '';
      draftFields.body = '';
    }
    function setCommittedField(field, value) {
      committedFields[field] = normalizeSpokenText(value || '');
    }
    function appendCommitted(field, spoken) {
      if (!spoken) {
        return;
      }
      committedFields[field] = [committedFields[field], spoken].join(' ').replace(/\s+/g, ' ').trim();
    }
    function parseTranscript(text, startingField) {
      var normalized = normalizeSpokenText(text);
      var parsed = {
        title: '',
        body: '',
        field: startingField,
        save: false
      };
      if (!normalized) {
        return parsed;
      }
      var field = startingField;
      var pattern = commandPattern();
      var lastIndex = 0;
      var match;
      while ((match = pattern.exec(normalized)) !== null) {
        var commandIndex = match.index + match[1].length;
        var spoken = normalizeSpokenText(normalized.slice(lastIndex, commandIndex));
        if (spoken) {
          parsed[field] = [parsed[field], spoken].join(' ').trim();
        }
        var command = normalizeSpokenText(match[2]).toLowerCase();
        lastIndex = commandIndex + match[2].length;
        if (command === normalizeSpokenText(audioSettings.wakeWord).toLowerCase()) {
          continue;
        }
        if (command === normalizeSpokenText(audioSettings.splitWord).toLowerCase()) {
          field = 'body';
          parsed.field = 'body';
          continue;
        }
        if (command === normalizeSpokenText(audioSettings.saveWord).toLowerCase()) {
          parsed.save = true;
          break;
        }
      }
      var tail = normalizeSpokenText(normalized.slice(lastIndex));
      if (tail) {
        parsed[field] = [parsed[field], tail].join(' ').trim();
      }
      parsed.field = field;
      return parsed;
    }
    function applyDraft(parsed) {
      draftFields.title = parsed.title;
      draftFields.body = parsed.body;
      renderFromState(parsed.field);
    }
    function commitParsed(parsed) {
      appendCommitted('title', parsed.title);
      appendCommitted('body', parsed.body);
      resetDraftFields();
      currentField = parsed.field;
      renderFromState(currentField);
    }
    function activateWakeMode() {
      wakeArmed = true;
      restartAfterWake = true;
      currentField = 'title';
      committedFields.title = '';
      committedFields.body = '';
      resetDraftFields();
      renderCaptureState();
      pulseHeard(micLiveDot);
      pulseHeard(titleDot);
      playStartTone();
      if (recognition && (recognitionActive || recognitionStarting)) {
        try {
          recognition.stop();
        } catch (err) {
        }
      }
    }
    function resetCaptureState() {
      wakeArmed = false;
      restartAfterWake = false;
      currentField = 'title';
      committedFields.title = '';
      committedFields.body = '';
      resetDraftFields();
      renderWakeListeningState();
    }
    function submitPromptFromAudio() {
      if (isSubmitting) {
        return;
      }
      isSubmitting = true;
      pauseMic();
      document.querySelector('form.form-grid').requestSubmit();
    }
    function ensureMicStream() {
      if (micStream) {
        return Promise.resolve(micStream);
      }
      if (micPermissionDenied || !navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        return Promise.resolve(null);
      }
      return navigator.mediaDevices.getUserMedia({ audio: true }).then(function(stream) {
        micStream = stream;
        return stream;
      }).catch(function() {
        micPermissionDenied = true;
        recognitionStarting = false;
        recognitionActive = false;
        shouldKeepListening = false;
        micStatus.textContent = 'microphone unavailable';
        setMicLive('');
        setActiveField('');
        return null;
      });
    }
    function releaseMicStream() {
      if (!micStream) {
        return;
      }
      micStream.getTracks().forEach(function(track) {
        track.stop();
      });
      micStream = null;
    }
    function ensureRecognition() {
      if (recognition) {
        return recognition;
      }
      var API = window.SpeechRecognition || window.webkitSpeechRecognition;
      if (!API) {
        micStatus.textContent = 'speech input unavailable';
        return null;
      }
      recognition = new API();
      recognition.continuous = true;
      recognition.interimResults = true;
      recognition.lang = 'en-US';
      recognition.maxAlternatives = 5;
      recognition.onstart = function() {
        recognitionStarting = false;
        recognitionActive = true;
        isSubmitting = false;
        if (wakeArmed) {
          renderCaptureState();
          return;
        }
        renderWakeListeningState();
      };
      recognition.onend = function() {
        recognitionStarting = false;
        recognitionActive = false;
        if (restartAfterWake && shouldKeepListening && !isSubmitting) {
          restartAfterWake = false;
          window.setTimeout(function() {
            startMic();
          }, 150);
          return;
        }
        if (shouldKeepListening && !isSubmitting) {
          window.setTimeout(function() {
            startMic();
          }, 150);
          return;
        }
        releaseMicStream();
        setMicLive('');
        if (!wakeArmed) {
          setActiveField('');
        }
        updateMicStatus();
      };
      recognition.onresult = function(event) {
        for (var i = event.resultIndex; i < event.results.length; i++) {
          var result = event.results[i];
          var transcript = normalizeSpokenText(result[0].transcript);
          if (!transcript) {
            continue;
          }
          pulseHeard(micLiveDot);
          if (!wakeArmed) {
            var wakeSplit = splitAfterWakeWordFromAlternatives(result);
            if (!wakeSplit) {
              setSpeechDebug('heard: "' + transcript + '"');
              continue;
            }
            activateWakeMode();
            setSpeechDebug('heard wake word in: "' + wakeSplit.transcript + '"');
            continue;
          }
          var parsed = parseTranscript(stripLeadingWakeWord(transcript), currentField);
          if (!result.isFinal) {
            setSpeechDebug('hearing: "' + transcript + '"');
            applyDraft(parsed);
            continue;
          }
          setSpeechDebug('captured: "' + transcript + '"');
          commitParsed(parsed);
          if (currentField === 'body') {
            pulseHeard(bodyDot);
          } else {
            pulseHeard(titleDot);
          }
          if (parsed.save) {
            submitPromptFromAudio();
            return;
          }
        }
      };
      recognition.onerror = function(event) {
        recognitionStarting = false;
        if (event && (event.error === 'not-allowed' || event.error === 'service-not-allowed' || event.error === 'audio-capture')) {
          shouldKeepListening = false;
          recognitionActive = false;
          micPermissionDenied = true;
          releaseMicStream();
          micStatus.textContent = 'microphone unavailable';
          setMicLive('');
          setActiveField('');
          return;
        }
        if (event && (event.error === 'aborted' || event.error === 'no-speech')) {
          return;
        }
        micStatus.textContent = 'speech error';
        setMicLive('');
      };
      return recognition;
    }
    function startMic() {
      var api = ensureRecognition();
      if (!api || recognitionActive || recognitionStarting) {
        return;
      }
      shouldKeepListening = true;
      micStatus.textContent = 'connecting microphone';
      ensureMicStream().then(function(stream) {
        if (!shouldKeepListening || !stream || recognitionActive || recognitionStarting) {
          return;
        }
        recognitionStarting = true;
        try {
          api.start();
        } catch (err) {
          recognitionStarting = false;
          micStatus.textContent = 'speech input busy';
        }
      });
    }
    function pauseMic() {
      shouldKeepListening = false;
      if (recognition && (recognitionActive || recognitionStarting)) {
        recognition.stop();
      } else {
        releaseMicStream();
      }
      recognitionStarting = false;
      recognitionActive = false;
      micStatus.textContent = 'paused';
      setMicLive('');
      setActiveField('');
    }
    function clearTranscript() {
      resetCaptureState();
      isSubmitting = false;
    }
    window.addEventListener('load', function() {
      renderWakeListeningState();
      window.setTimeout(function() {
        startMic();
      }, 250);
    });
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var prefixTemplate = template.Must(template.New("prefix").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp prefix</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <div class="actions spaced">
          <button type="button" class="icon-button" onclick="startPrefixMic()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15a3 3 0 0 0 3-3V7a3 3 0 0 0-6 0v5a3 3 0 0 0 3 3zm5-3a1 1 0 0 1 2 0 7 7 0 0 1-6 6.93V21h3a1 1 0 0 1 0 2H8a1 1 0 0 1 0-2h3v-2.07A7 7 0 0 1 5 12a1 1 0 0 1 2 0 5 5 0 0 0 10 0z"/></svg>
            <span>Mic</span>
          </button>
          <button type="button" class="icon-button" onclick="pausePrefixMic()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7 5a1 1 0 0 1 1 1v12a1 1 0 1 1-2 0V6a1 1 0 0 1 1-1zm10 0a1 1 0 0 1 1 1v12a1 1 0 1 1-2 0V6a1 1 0 0 1 1-1z"/></svg>
            <span>Pause</span>
          </button>
          <button type="button" class="icon-button" onclick="clearPrefixMic()">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 3h6l1 2h4a1 1 0 1 1 0 2h-1l-1 12a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 7H4a1 1 0 1 1 0-2h4l1-2zm1.2 4L8 19h8L13.8 7h-3.6z"/></svg>
            <span>Clear text</span>
          </button>
        </div>
        <div class="compact">
          <div id="prefix-mic-status" class="muted mic-status"></div>
          <span id="prefix-mic-live-dot" class="dot" aria-hidden="true"></span>
        </div>
        <label class="label">
          <span>Prefix</span>
          <textarea id="prefix-input" name="prefix">{{.Prefix}}</textarea>
        </label>
        <div class="actions spaced">
          <button type="submit" class="primary">Save prefix</button>
        </div>
      </form>
    </section>
  </main>
  <script>
    var prefixRecognition = null;
    var prefixRecognitionStarting = false;
    var prefixRecognitionRunning = false;
    var prefixMicStream = null;
    var prefixMicPermissionDenied = false;
    var prefixTranscript = '';
    var prefixDraft = '';
    var prefixShouldKeepListening = false;
    var prefixMicStatus = document.getElementById('prefix-mic-status');
    var prefixMicLiveDot = document.getElementById('prefix-mic-live-dot');
    function playPrefixStartTone() {
      var AudioContextCtor = window.AudioContext || window.webkitAudioContext;
      if (!AudioContextCtor) {
        return;
      }
      try {
        var ctx = new AudioContextCtor();
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.type = 'sine';
        osc.frequency.value = 880;
        gain.gain.value = 0.03;
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.start();
        gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.12);
        osc.stop(ctx.currentTime + 0.12);
        osc.onended = function() {
          ctx.close();
        };
      } catch (err) {
      }
    }
    function setPrefixMicLive(live) {
      prefixMicLiveDot.classList.toggle('live', live);
    }
    function ensurePrefixMicStream() {
      if (prefixMicStream) {
        return Promise.resolve(prefixMicStream);
      }
      if (prefixMicPermissionDenied || !navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        return Promise.resolve(null);
      }
      return navigator.mediaDevices.getUserMedia({ audio: true }).then(function(stream) {
        prefixMicStream = stream;
        return stream;
      }).catch(function(err) {
        prefixMicPermissionDenied = true;
        prefixMicStatus.textContent = 'microphone unavailable';
        setPrefixMicLive(false);
        return null;
      });
    }
    function releasePrefixMicStream() {
      if (!prefixMicStream) {
        return;
      }
      prefixMicStream.getTracks().forEach(function(track) {
        track.stop();
      });
      prefixMicStream = null;
    }
    function updatePrefixField() {
      document.getElementById('prefix-input').value = (prefixTranscript + ' ' + prefixDraft).replace(/\s+/g, ' ').trim();
    }
    function ensurePrefixRecognition() {
      if (prefixRecognition) {
        return prefixRecognition;
      }
      var API = window.SpeechRecognition || window.webkitSpeechRecognition;
      if (!API) {
        prefixMicStatus.textContent = 'speech input unavailable';
        return null;
      }
      prefixRecognition = new API();
      prefixRecognition.continuous = true;
      prefixRecognition.interimResults = true;
      prefixRecognition.onresult = function(event) {
        var finalParts = [];
        var interimParts = [];
        for (var i = 0; i < event.results.length; i++) {
          var transcript = event.results[i][0].transcript;
          if (event.results[i].isFinal) {
            finalParts.push(transcript);
          } else {
            interimParts.push(transcript);
          }
        }
        prefixTranscript = finalParts.join(' ').trim();
        prefixDraft = interimParts.join(' ').trim();
        updatePrefixField();
      };
      prefixRecognition.onstart = function() {
        prefixRecognitionStarting = false;
        prefixRecognitionRunning = true;
        prefixMicStatus.textContent = 'listening';
        setPrefixMicLive(true);
        playPrefixStartTone();
      };
      prefixRecognition.onend = function() {
        prefixRecognitionStarting = false;
        prefixRecognitionRunning = false;
        if (prefixMicStatus.textContent === 'listening') {
          prefixMicStatus.textContent = 'paused';
        }
        if (!prefixShouldKeepListening) {
          releasePrefixMicStream();
        }
        setPrefixMicLive(false);
      };
      prefixRecognition.onerror = function(event) {
        if (event && (event.error === 'not-allowed' || event.error === 'service-not-allowed')) {
          prefixShouldKeepListening = false;
          prefixMicPermissionDenied = true;
          releasePrefixMicStream();
          prefixMicStatus.textContent = 'microphone unavailable';
          setPrefixMicLive(false);
          return;
        }
        prefixMicStatus.textContent = 'speech error';
        setPrefixMicLive(false);
      };
      return prefixRecognition;
    }
    function startPrefixMic() {
      var api = ensurePrefixRecognition();
      if (!api) {
        return;
      }
      prefixShouldKeepListening = true;
      if (prefixRecognitionRunning || prefixRecognitionStarting) {
        return;
      }
      prefixMicStatus.textContent = 'connecting microphone';
      ensurePrefixMicStream().then(function(stream) {
        if (!prefixShouldKeepListening || !stream || prefixRecognitionRunning || prefixRecognitionStarting) {
          return;
        }
        prefixRecognitionStarting = true;
        try {
          api.start();
        } catch (err) {
          prefixRecognitionStarting = false;
          prefixMicStatus.textContent = 'speech input busy';
        }
      });
    }
    function pausePrefixMic() {
      if (!prefixRecognition) {
        prefixShouldKeepListening = false;
        releasePrefixMicStream();
        return;
      }
      prefixShouldKeepListening = false;
      if (prefixRecognitionRunning || prefixRecognitionStarting) {
        prefixRecognition.stop();
      } else {
        releasePrefixMicStream();
      }
      prefixMicStatus.textContent = 'paused';
    }
    function clearPrefixMic() {
      prefixTranscript = '';
      prefixDraft = '';
      document.getElementById('prefix-input').value = '';
      prefixMicStatus.textContent = '';
      setPrefixMicLive(false);
    }
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var settingsTemplate = template.Must(template.New("settings").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp settings</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <label class="label">
          <span>Wake word</span>
          <input type="text" name="wake_word" value="{{.AudioSettings.WakeWord}}">
        </label>
        <label class="label">
          <span>Split word</span>
          <input type="text" name="split_word" value="{{.AudioSettings.SplitWord}}">
        </label>
        <label class="label">
          <span>Save word</span>
          <input type="text" name="save_word" value="{{.AudioSettings.SaveWord}}">
        </label>
        <div class="actions spaced">
          <button type="submit" class="primary">Save settings</button>
        </div>
      </form>
    </section>
  </main>
  ` + liveReloadScript + `
</body>
</html>`))

var promptsTemplate = template.Must(template.New("prompts").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp prompts</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    <section class="panel compact">
      <div>{{.TotalPrompts}} prompts · page {{.Page}}/{{.TotalPages}}</div>
      <div class="actions">
        <button type="button" onclick="toggleAll(true)">Open all</button>
        <button type="button" onclick="toggleAll(false)">Close all</button>
      </div>
    </section>
    <section class="stack">
      {{range .Prompts}}
      <details id="{{.ElementID}}" {{if .Marked}}class="marked"{{end}}>
        <summary>
          <div class="summary-row">
            <span class="summary-title">[{{.Index}}] {{.Title}}{{if .Marked}}<span class="mark-badge">marked</span>{{end}}</span>
            <span class="summary-meta">{{.Timestamp}}</span>
          </div>
        </summary>
        <article>{{.HTMLBody}}</article>
        <div class="prompt-actions">
          <form method="post" action="/prompts/delete">
            <input type="hidden" name="index" value="{{.Index}}">
            <button type="submit" class="danger">Delete</button>
          </form>
        </div>
      </details>
      {{else}}
      <section class="panel">
        <div class="muted">No prompts.</div>
      </section>
      {{end}}
    </section>
    <div class="pager">
      <div>{{if .PrevPage}}<a class="button" href="/prompts?page={{.PrevPage}}">Newer</a>{{end}}</div>
      <div>{{if .NextPage}}<a class="button" href="/prompts?page={{.NextPage}}">Older</a>{{end}}</div>
    </div>
  </main>
  <script>
    function toggleAll(open) {
      document.querySelectorAll("details").forEach(function(el) {
        el.open = open;
      });
    }
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var compileTemplate = template.Must(template.New("compile").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp compile</title>
  <style>` + baseStyles + `</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      {{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}
    </nav>
    <section class="panel compact">
      <div>{{.TotalPrompts}} prompts</div>
      <div class="actions">
        <button type="button" onclick="setAll(true)">Select all</button>
        {{if .HasMark}}<button type="button" onclick="selectFromMark()">Select from mark</button>{{end}}
        <button type="button" onclick="setAll(false)">Clear all</button>
      </div>
    </section>
    {{if .Error}}
    <section class="panel error">{{.Error}}</section>
    {{end}}
    {{if .Copied}}
    <section class="panel">compiled to clipboard</section>
    {{end}}
    <section class="panel">
      <form id="compile-form" method="post">
        <div class="actions spaced">
          <button type="submit" class="primary">Compile</button>
        </div>
        <div class="prompt-picker" {{if .HasMark}}data-marked-index="{{.MarkedIndex}}"{{end}}>
          {{range .Options}}
          <label class="prompt-option {{if .Marked}}marked{{end}}">
            <input type="checkbox" name="prompt" value="{{.Index}}" data-index="{{.Index}}" {{if .Checked}}checked{{end}}>
            <span>
              <strong>[{{.Index}}] {{.Title}}</strong>{{if .Marked}} <span class="mark-badge">marked</span>{{end}}<br>
              <span class="small">{{.Timestamp}}</span>
            </span>
          </label>
          {{else}}
          <div class="muted">No prompts.</div>
          {{end}}
        </div>
      </form>
    </section>
  </main>
  <script>
    function setAll(checked) {
      document.querySelectorAll('input[name="prompt"]').forEach(function(el) {
        el.checked = checked;
      });
    }
    function selectFromMark() {
      var picker = document.querySelector('.prompt-picker');
      if (!picker) {
        return;
      }
      var markedIndex = parseInt(picker.getAttribute('data-marked-index'), 10);
      if (Number.isNaN(markedIndex)) {
        return;
      }
      document.querySelectorAll('input[name="prompt"]').forEach(function(el) {
        var index = parseInt(el.getAttribute('data-index'), 10);
        el.checked = !Number.isNaN(index) && index > markedIndex;
      });
    }
    document.getElementById('compile-form').addEventListener('submit', function(event) {
      event.preventDefault();
      var form = event.currentTarget;
      var params = new URLSearchParams();
      form.querySelectorAll('input[name="prompt"]:checked').forEach(function(el) {
        params.append('prompt', el.value);
      });
      if (!params.has('prompt')) {
        var emptyError = document.querySelector('.error');
        if (!emptyError) {
          emptyError = document.createElement('section');
          emptyError.className = 'panel error';
          form.parentNode.parentNode.insertBefore(emptyError, form.parentNode);
        }
        emptyError.textContent = 'select at least one prompt to compile';
        return;
      }
      fetch('/compile', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8'
        },
        body: params.toString()
      }).then(function(response) {
        return response.json();
      }).then(function(payload) {
        if (payload.error) {
          var error = document.querySelector('.error');
          if (!error) {
            error = document.createElement('section');
            error.className = 'panel error';
            form.parentNode.parentNode.insertBefore(error, form.parentNode);
          }
          error.textContent = payload.error;
          return;
        }
        var text = payload.compiled || '';
        function fallbackCopy() {
          var area = document.createElement('textarea');
          area.value = text;
          document.body.appendChild(area);
          area.focus();
          area.select();
          try {
            document.execCommand('copy');
          } catch (err) {
          }
          document.body.removeChild(area);
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).catch(fallbackCopy).finally(function() {
            window.location.href = '/compile?copied=1';
          });
        } else {
          fallbackCopy();
          window.location.href = '/compile?copied=1';
        }
      });
    });
  </script>
  ` + liveReloadScript + `
</body>
</html>`))
