package engine

import (
	"fmt"
	"regexp"
	"strings"
)

// ══════════════════════════════════════════════════════════════════════════
// Pipeline DSL interpreter
//
// A minimal line-oriented DSL for extracting structured data from CLI output.
// Instructions are one-per-line, verb first, space-separated arguments.
//
// ─── Phase 1: Trimming ───
//   SKIP_UNTIL  <regex>      skip lines until regex matches (inclusive)
//   SKIP_LINES  <N>          skip N lines after the previous instruction
//   SKIP_BLANK               skip blank lines
//   STOP_AT     <regex>      stop processing when regex matches
//   FILTER      <regex>      keep only lines matching regex
//   REJECT      <regex>      discard lines matching regex
//
// ─── Phase 2: Extraction ───
//   SPLIT       $a $b $c     whitespace-split, assign to variables; last var gets rest
//   REGEX       <pattern>    named groups (?P<name>...) extract variables
//   REPLACE     <old> <new>  text replacement on current line (before extraction)
//
// ─── Phase 3: Post-processing ───
//   SET         $name <expr> assign: literal "val", concat $a "/" $b, or ternary $a == "x" ? "y" : "z"
//   EMIT                     explicit emit (auto-emit per line if omitted)
//
// ─── Multi-section (for multi-table output) ───
//   SECTION                  start a new independent processing section;
//                            each section runs its own trimming + extraction against
//                            the full raw input, results are joined by row index
//
// ─── Repeating groups (for parent-child structures) ───
//   REPEAT_FOR  <regex>      split input into blocks at lines matching regex;
//                            named groups (?P<name>...) in the regex are extracted
//                            as "parent" fields and broadcast to every child row
//                            within that block. Subsequent instructions (FILTER,
//                            SPLIT, REGEX, etc.) run independently on each block's
//                            child lines, and all blocks' rows are concatenated.
//                            Supports nesting: a second REPEAT_FOR after the first
//                            further splits each block into sub-blocks (recursive).
//
// Two execution modes (auto-detected):
//   Table mode  — when SPLIT is present: each data line → one output row
//   Record mode — when only REGEX: whole text → one record (multiple REGEX merge)
// ══════════════════════════════════════════════════════════════════════════

// PipelineResult holds the output of ExecPipeline.
type PipelineResult struct {
	Rows    []map[string]string `json:"rows"`
	Columns []string            `json:"columns"` // ordered column names
	Mode    string              `json:"mode"`     // "table" or "record"
}

// ValidatePipelineDSL parses the DSL text and checks for syntax errors without
// executing it against any input.  Returns nil when the DSL is syntactically valid.
func ValidatePipelineDSL(dsl string) error {
	instructions, err := parseDSL(dsl)
	if err != nil {
		return fmt.Errorf("DSL parse error: %w", err)
	}
	if len(instructions) == 0 {
		return fmt.Errorf("pipeline: empty DSL program")
	}

	validVerbs := map[string]bool{
		"SKIP_UNTIL": true, "SKIP_LINES": true, "SKIP_BLANK": true,
		"STOP_AT": true, "FILTER": true, "REJECT": true,
		"SPLIT": true, "REGEX": true, "REPLACE": true,
		"SET": true, "EMIT": true, "SECTION": true, "REPEAT_FOR": true,
	}

	for _, inst := range instructions {
		if !validVerbs[inst.verb] {
			return fmt.Errorf("line %d: unknown verb %q", inst.line, inst.verb)
		}

		// Validate regex-bearing instructions compile
		switch inst.verb {
		case "SKIP_UNTIL", "STOP_AT", "FILTER", "REJECT":
			if len(inst.args) == 0 {
				return fmt.Errorf("line %d: %s requires a regex argument", inst.line, inst.verb)
			}
			if _, err := regexp.Compile(inst.args[0]); err != nil {
				return fmt.Errorf("line %d: %s has invalid regex %q: %w", inst.line, inst.verb, inst.args[0], err)
			}
		case "REGEX", "REPEAT_FOR":
			if len(inst.args) == 0 {
				return fmt.Errorf("line %d: %s requires a regex argument", inst.line, inst.verb)
			}
			if _, err := regexp.Compile(inst.args[0]); err != nil {
				return fmt.Errorf("line %d: %s has invalid regex %q: %w", inst.line, inst.verb, inst.args[0], err)
			}
		case "SPLIT":
			if len(inst.args) == 0 {
				return fmt.Errorf("line %d: SPLIT requires at least one variable name", inst.line)
			}
		case "REPLACE":
			if len(inst.args) < 2 {
				return fmt.Errorf("line %d: REPLACE requires <pattern> <replacement>", inst.line)
			}
			if _, err := regexp.Compile(inst.args[0]); err != nil {
				return fmt.Errorf("line %d: REPLACE has invalid regex %q: %w", inst.line, inst.args[0], err)
			}
		case "SKIP_LINES":
			if len(inst.args) == 0 {
				return fmt.Errorf("line %d: SKIP_LINES requires a number", inst.line)
			}
		case "SET":
			if len(inst.args) < 2 {
				return fmt.Errorf("line %d: SET requires $name <expr>", inst.line)
			}
		}
	}
	return nil
}

// instruction is a parsed DSL instruction.
type instruction struct {
	verb string   // e.g. "SKIP_UNTIL", "SPLIT", "REGEX", ...
	args []string // remaining tokens (raw)
	raw  string   // original line for error messages
	line int      // 1-based line number in DSL
}

// ExecPipeline interprets a DSL program against raw CLI output and returns
// structured rows. The DSL text and raw input are both plain strings.
//
// When the DSL contains SECTION directives, the program is split into
// independent sub-pipelines. Each section processes the raw input separately
// with its own trimming/extraction. Results are joined by row index into a
// wide table (columns from all sections merged).
func ExecPipeline(dsl string, raw string) (PipelineResult, error) {
	instructions, err := parseDSL(dsl)
	if err != nil {
		return PipelineResult{}, err
	}
	if len(instructions) == 0 {
		return PipelineResult{}, fmt.Errorf("pipeline: empty DSL program")
	}

	// Check if DSL uses SECTION directives
	sections := splitSections(instructions)
	if len(sections) > 1 {
		return execSectionMode(sections, raw)
	}

	// Check if DSL uses REPEAT_FOR directive
	for _, inst := range instructions {
		if inst.verb == "REPEAT_FOR" {
			return execRepeatForMode(instructions, raw)
		}
	}

	// Single-section: detect mode as before
	mode := "record"
	for _, inst := range instructions {
		if inst.verb == "SPLIT" {
			mode = "table"
			break
		}
	}

	if mode == "table" {
		return execTableMode(instructions, raw)
	}
	return execRecordMode(instructions, raw)
}

// splitSections splits instructions at SECTION boundaries.
// If no SECTION instructions exist, returns a single slice containing all instructions.
// Instructions before the first SECTION are included as the first section.
func splitSections(insts []instruction) [][]instruction {
	var sections [][]instruction
	var current []instruction
	for _, inst := range insts {
		if inst.verb == "SECTION" {
			if len(current) > 0 {
				sections = append(sections, current)
			}
			current = nil
			continue
		}
		current = append(current, inst)
	}
	if len(current) > 0 {
		sections = append(sections, current)
	}
	return sections
}

// execSectionMode runs each section independently against the raw input,
// then joins results by row index into a single wide table.
func execSectionMode(sections [][]instruction, raw string) (PipelineResult, error) {
	var allColumns []string
	var sectionResults []PipelineResult

	for i, sectionInsts := range sections {
		// Detect mode for this section
		mode := "record"
		for _, inst := range sectionInsts {
			if inst.verb == "SPLIT" {
				mode = "table"
				break
			}
		}

		var result PipelineResult
		var err error
		if mode == "table" {
			result, err = execTableMode(sectionInsts, raw)
		} else {
			result, err = execRecordMode(sectionInsts, raw)
		}
		if err != nil {
			return PipelineResult{}, fmt.Errorf("section %d: %w", i+1, err)
		}

		sectionResults = append(sectionResults, result)
		allColumns = append(allColumns, result.Columns...)
	}

	// Determine the maximum number of rows across all sections
	maxRows := 0
	for _, sr := range sectionResults {
		if len(sr.Rows) > maxRows {
			maxRows = len(sr.Rows)
		}
	}
	if maxRows == 0 {
		return PipelineResult{Rows: nil, Columns: allColumns, Mode: "table"}, nil
	}

	// Join rows by index: row[i] = merge of all sections' row[i]
	rows := make([]map[string]string, maxRows)
	for i := 0; i < maxRows; i++ {
		merged := make(map[string]string, len(allColumns))
		for _, sr := range sectionResults {
			if i < len(sr.Rows) {
				for k, v := range sr.Rows[i] {
					merged[k] = v
				}
			}
		}
		rows[i] = merged
	}

	return PipelineResult{
		Rows:    rows,
		Columns: allColumns,
		Mode:    "table",
	}, nil
}

// execRepeatForMode handles DSLs with REPEAT_FOR directive.
//
// The input is split into "blocks" at lines matching the REPEAT_FOR pattern.
// Named groups in the pattern (e.g. (?P<acl_number>\d+)) are extracted as
// "parent" fields and broadcast to every child row within that block.
// Remaining instructions (FILTER, REGEX, SPLIT, etc.) run independently
// on each block's child lines, and all blocks' rows are concatenated.
func execRepeatForMode(insts []instruction, raw string) (PipelineResult, error) {
	// Find the REPEAT_FOR instruction and separate pre/post instructions
	var preInsts []instruction  // instructions before REPEAT_FOR (global trimming)
	var postInsts []instruction // instructions after REPEAT_FOR (per-block extraction)
	var repeatInst *instruction
	found := false
	for i := range insts {
		if insts[i].verb == "REPEAT_FOR" && !found {
			inst := insts[i]
			repeatInst = &inst
			found = true
			continue
		}
		if !found {
			preInsts = append(preInsts, insts[i])
		} else {
			postInsts = append(postInsts, insts[i])
		}
	}

	if repeatInst == nil || len(repeatInst.args) == 0 {
		return PipelineResult{}, fmt.Errorf("pipeline: REPEAT_FOR requires a regex pattern")
	}

	repeatRe, err := regexp.Compile(repeatInst.args[0])
	if err != nil {
		return PipelineResult{}, fmt.Errorf("pipeline line %d: invalid regex %q: %w",
			repeatInst.line, repeatInst.args[0], err)
	}

	// Phase 1: Apply global pre-trimming (e.g., STOP_AT for device prompt lines)
	lines := strings.Split(raw, "\n")
	if len(preInsts) > 0 {
		// Only apply global STOP_AT and REJECT from pre-instructions
		var globalTrimInsts []instruction
		for _, inst := range preInsts {
			switch inst.verb {
			case "STOP_AT", "REJECT":
				globalTrimInsts = append(globalTrimInsts, inst)
			}
		}
		if len(globalTrimInsts) > 0 {
			lines, err = applyTrimming(globalTrimInsts, lines)
			if err != nil {
				return PipelineResult{}, err
			}
		}
	}

	// Phase 2: Split lines into blocks at REPEAT_FOR boundaries
	type block struct {
		parentFields map[string]string // named groups from the REPEAT_FOR match
		childLines   []string          // lines between this header and the next
	}
	var blocks []block

	for _, line := range lines {
		m := repeatRe.FindStringSubmatch(line)
		if m != nil {
			// Start a new block; extract parent fields from named groups
			parent := make(map[string]string)
			for i, name := range repeatRe.SubexpNames() {
				if i == 0 || name == "" {
					continue
				}
				parent[name] = m[i]
			}
			blocks = append(blocks, block{parentFields: parent})
			continue
		}
		// Append to the current block's child lines
		if len(blocks) > 0 {
			blocks[len(blocks)-1].childLines = append(blocks[len(blocks)-1].childLines, line)
		}
		// Lines before the first REPEAT_FOR match are discarded
	}

	if len(blocks) == 0 {
		return PipelineResult{Rows: nil, Columns: nil, Mode: "table"}, nil
	}

	// Determine parent column names (in order of named groups)
	var parentCols []string
	parentColSet := make(map[string]bool)
	for _, name := range repeatRe.SubexpNames() {
		if name != "" && !parentColSet[name] {
			parentCols = append(parentCols, name)
			parentColSet[name] = true
		}
	}

	// Phase 3: For each block, run the post-instructions to extract child rows
	var allRows []map[string]string
	var childCols []string
	childColSet := make(map[string]bool)
	childColsSet := false

	for _, blk := range blocks {
		blockRaw := strings.Join(blk.childLines, "\n")

		// Detect mode for post-instructions
		hasSplit := false
		hasFilter := false
		hasRegex := false
		hasNestedRepeat := false
		for _, inst := range postInsts {
			switch inst.verb {
			case "SPLIT":
				hasSplit = true
			case "FILTER":
				hasFilter = true
			case "REGEX":
				hasRegex = true
			case "REPEAT_FOR":
				hasNestedRepeat = true
			}
		}

		var result PipelineResult
		if hasNestedRepeat {
			// Nested REPEAT_FOR: recursively split the block into sub-blocks
			result, err = execRepeatForMode(postInsts, blockRaw)
		} else if hasSplit {
			result, err = execTableMode(postInsts, blockRaw)
		} else if hasFilter && hasRegex {
			// FILTER + REGEX without SPLIT → regex table mode (one row per filtered line)
			result, err = execRegexTableMode(postInsts, blockRaw)
		} else {
			result, err = execRecordMode(postInsts, blockRaw)
		}
		if err != nil {
			return PipelineResult{}, fmt.Errorf("REPEAT_FOR block: %w", err)
		}

		// Accumulate child columns from all blocks (union, preserving order)
		for _, col := range result.Columns {
			if !childColsSet || !childColSet[col] {
				childCols = append(childCols, col)
				childColSet[col] = true
			}
		}
		if len(result.Columns) > 0 {
			childColsSet = true
		}

		// Broadcast parent fields to each child row
		for _, row := range result.Rows {
			merged := make(map[string]string, len(parentCols)+len(row))
			for k, v := range blk.parentFields {
				merged[k] = v
			}
			for k, v := range row {
				merged[k] = v
			}
			allRows = append(allRows, merged)
		}
	}

	// Build column list: parent cols first, then child cols
	allCols := make([]string, 0, len(parentCols)+len(childCols))
	allCols = append(allCols, parentCols...)
	allCols = append(allCols, childCols...)

	return PipelineResult{
		Rows:    allRows,
		Columns: allCols,
		Mode:    "table",
	}, nil
}

// ── DSL Parser ───────────────────────────────────────────────────────────

func parseDSL(dsl string) ([]instruction, error) {
	var insts []instruction
	for i, rawLine := range strings.Split(dsl, "\n") {
		line := strings.TrimSpace(rawLine)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract verb (first whitespace-separated token)
		verb, rest := splitFirst(line)
		if verb == "" {
			continue
		}
		verb = strings.ToUpper(verb)

		// Parse args based on verb type:
		// - Single-arg verbs (regex pattern): SKIP_UNTIL, STOP_AT, FILTER, REJECT, REGEX
		//   → entire rest is one argument (the regex pattern)
		// - Two-arg verbs: REPLACE <pattern> <replacement>
		//   → split rest into two tokens (respecting quotes)
		// - Multi-arg verbs: SPLIT $a $b $c, SET $name <expr parts>, SKIP_LINES <N>
		//   → whitespace-split all remaining tokens
		// - No-arg verbs: SKIP_BLANK, EMIT
		var args []string
		switch verb {
		case "SKIP_UNTIL", "STOP_AT", "FILTER", "REJECT", "REGEX", "REPEAT_FOR":
			// Strip inline comment (only outside the pattern)
			// For regex patterns, we take the whole rest as-is
			rest = strings.TrimSpace(rest)
			if rest != "" {
				args = []string{rest}
			}
		case "REPLACE":
			// Two arguments: <pattern> <replacement>
			// Use quote-aware splitting
			args = splitDSLLine(rest)
		case "SKIP_BLANK", "EMIT", "SECTION":
			// No arguments
		default:
			// SPLIT, SET, SKIP_LINES, and any future verbs: whitespace-split
			args = splitDSLLine(rest)
		}

		insts = append(insts, instruction{
			verb: verb,
			args: args,
			raw:  rawLine,
			line: i + 1,
		})
	}
	return insts, nil
}

// splitFirst splits s into the first whitespace-delimited token and the rest.
func splitFirst(s string) (first, rest string) {
	s = strings.TrimSpace(s)
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}

// splitDSLLine splits a DSL line respecting quoted strings.
// Tokens are separated by whitespace. Quoted strings ("...") are kept as
// single tokens with quotes stripped. An empty quoted string "" yields an
// empty-string token (important for REPLACE with empty replacement).
func splitDSLLine(line string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	hasQuote := false // tracks whether current token started with a quote

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
				// Don't add the closing quote
			} else {
				current.WriteByte(ch)
			}
		} else if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			hasQuote = true
			// Don't add the opening quote
		} else if ch == ' ' || ch == '\t' {
			if current.Len() > 0 || hasQuote {
				tokens = append(tokens, current.String())
				current.Reset()
				hasQuote = false
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 || hasQuote {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// ── Table Mode Execution ─────────────────────────────────────────────────

func execTableMode(insts []instruction, raw string) (PipelineResult, error) {
	lines := strings.Split(raw, "\n")

	// Separate instructions into phases
	var (
		trimInsts    []instruction // SKIP_UNTIL, SKIP_LINES, SKIP_BLANK, STOP_AT, FILTER, REJECT
		replaceInsts []instruction // REPLACE (applied per line before SPLIT)
		splitInst    *instruction  // the SPLIT instruction
		setInsts     []instruction // SET instructions
		hasEmit      bool
	)

	for i := range insts {
		switch insts[i].verb {
		case "SKIP_UNTIL", "SKIP_LINES", "SKIP_BLANK", "STOP_AT", "FILTER", "REJECT":
			trimInsts = append(trimInsts, insts[i])
		case "REPLACE":
			replaceInsts = append(replaceInsts, insts[i])
		case "SPLIT":
			inst := insts[i]
			splitInst = &inst
		case "SET":
			setInsts = append(setInsts, insts[i])
		case "EMIT":
			hasEmit = true
		case "REGEX":
			// In table mode, REGEX can also be used (e.g., REGEX after SPLIT for extra extraction)
			// For now, ignore REGEX in table mode — SPLIT is the primary extractor
		default:
			return PipelineResult{}, fmt.Errorf("pipeline line %d: unknown verb %q", insts[i].line, insts[i].verb)
		}
	}

	if splitInst == nil {
		return PipelineResult{}, fmt.Errorf("pipeline: table mode detected but no SPLIT instruction found")
	}

	// Phase 1: Apply trimming instructions to filter lines
	dataLines, err := applyTrimming(trimInsts, lines)
	if err != nil {
		return PipelineResult{}, err
	}

	// Get column names from SPLIT + SET
	splitVars := splitInst.args
	if len(splitVars) == 0 {
		return PipelineResult{}, fmt.Errorf("pipeline line %d: SPLIT requires at least one $variable", splitInst.line)
	}
	columns := make([]string, len(splitVars))
	colSet := make(map[string]bool, len(splitVars))
	for i, v := range splitVars {
		name := strings.TrimPrefix(v, "$")
		columns[i] = name
		colSet[name] = true
	}
	// Also include any new columns introduced by SET instructions
	for _, si := range setInsts {
		if len(si.args) >= 1 {
			name := strings.TrimPrefix(si.args[0], "$")
			if !colSet[name] {
				columns = append(columns, name)
				colSet[name] = true
			}
		}
	}

	// Phase 2+3: Process each data line
	_ = hasEmit // EMIT is optional — in table mode, every line auto-emits
	var rows []map[string]string
	for _, line := range dataLines {
		// Apply REPLACE instructions
		processed := line
		for _, ri := range replaceInsts {
			var err error
			processed, err = applyReplace(ri, processed)
			if err != nil {
				return PipelineResult{}, err
			}
		}

		// Skip blank lines after replacement
		if strings.TrimSpace(processed) == "" {
			continue
		}

		// SPLIT
		row := applySplit(columns, processed)

		// SET
		for _, si := range setInsts {
			if err := applySet(si, row); err != nil {
				return PipelineResult{}, err
			}
		}

		rows = append(rows, row)
	}

	return PipelineResult{
		Rows:    rows,
		Columns: columns,
		Mode:    "table",
	}, nil
}

// ── Record Mode Execution ────────────────────────────────────────────────

func execRecordMode(insts []instruction, raw string) (PipelineResult, error) {
	lines := strings.Split(raw, "\n")

	// Separate instructions
	var (
		trimInsts    []instruction
		replaceInsts []instruction
		regexInsts   []instruction
		setInsts     []instruction
	)

	for i := range insts {
		switch insts[i].verb {
		case "SKIP_UNTIL", "SKIP_LINES", "SKIP_BLANK", "STOP_AT", "FILTER", "REJECT":
			trimInsts = append(trimInsts, insts[i])
		case "REPLACE":
			replaceInsts = append(replaceInsts, insts[i])
		case "REGEX":
			regexInsts = append(regexInsts, insts[i])
		case "SET":
			setInsts = append(setInsts, insts[i])
		case "EMIT":
			// In record mode, EMIT is implicit at the end
		default:
			return PipelineResult{}, fmt.Errorf("pipeline line %d: unknown verb %q", insts[i].line, insts[i].verb)
		}
	}

	// Phase 1: Trim
	dataLines, err := applyTrimming(trimInsts, lines)
	if err != nil {
		return PipelineResult{}, err
	}

	// Apply REPLACE to all lines, then join back
	text := strings.Join(dataLines, "\n")
	for _, ri := range replaceInsts {
		text, err = applyReplace(ri, text)
		if err != nil {
			return PipelineResult{}, err
		}
	}

	// Phase 2: Apply each REGEX — all named groups merge into one record
	record := make(map[string]string)
	var columns []string
	for _, ri := range regexInsts {
		if len(ri.args) == 0 {
			return PipelineResult{}, fmt.Errorf("pipeline line %d: REGEX requires a pattern", ri.line)
		}
		pattern := ri.args[0]
		re, err := regexp.Compile(pattern)
		if err != nil {
			return PipelineResult{}, fmt.Errorf("pipeline line %d: invalid regex %q: %w", ri.line, pattern, err)
		}

		// Try to match against each line (for multiline text, match per line)
		for _, line := range strings.Split(text, "\n") {
			m := re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			names := re.SubexpNames()
			for i, name := range names {
				if i == 0 || name == "" {
					continue
				}
				if _, exists := record[name]; !exists {
					columns = append(columns, name)
				}
				record[name] = m[i]
			}
			break // Use first match for each REGEX
		}
	}

	// Phase 3: SET
	for _, si := range setInsts {
		if err := applySet(si, record); err != nil {
			return PipelineResult{}, err
		}
	}

	// If no data was extracted, return empty
	if len(record) == 0 {
		return PipelineResult{Rows: nil, Columns: columns, Mode: "record"}, nil
	}

	return PipelineResult{
		Rows:    []map[string]string{record},
		Columns: columns,
		Mode:    "record",
	}, nil
}

// ── Regex Table Mode Execution ───────────────────────────────────────────
// When FILTER + REGEX are used together (no SPLIT), each filtered line produces
// one output row. This is a hybrid between table mode and record mode.

func execRegexTableMode(insts []instruction, raw string) (PipelineResult, error) {
	lines := strings.Split(raw, "\n")

	var (
		trimInsts    []instruction
		replaceInsts []instruction
		regexInsts   []instruction
		setInsts     []instruction
	)

	for i := range insts {
		switch insts[i].verb {
		case "SKIP_UNTIL", "SKIP_LINES", "SKIP_BLANK", "STOP_AT", "FILTER", "REJECT":
			trimInsts = append(trimInsts, insts[i])
		case "REPLACE":
			replaceInsts = append(replaceInsts, insts[i])
		case "REGEX":
			regexInsts = append(regexInsts, insts[i])
		case "SET":
			setInsts = append(setInsts, insts[i])
		case "EMIT":
			// ignored
		default:
			return PipelineResult{}, fmt.Errorf("pipeline line %d: unknown verb %q", insts[i].line, insts[i].verb)
		}
	}

	// Phase 1: Trim
	dataLines, err := applyTrimming(trimInsts, lines)
	if err != nil {
		return PipelineResult{}, err
	}

	// Pre-compile all regex patterns and collect column names
	type compiledRegex struct {
		re   *regexp.Regexp
		inst instruction
	}
	var compiledRegexes []compiledRegex
	var columns []string
	colSet := make(map[string]bool)

	for _, ri := range regexInsts {
		if len(ri.args) == 0 {
			return PipelineResult{}, fmt.Errorf("pipeline line %d: REGEX requires a pattern", ri.line)
		}
		re, err := regexp.Compile(ri.args[0])
		if err != nil {
			return PipelineResult{}, fmt.Errorf("pipeline line %d: invalid regex %q: %w", ri.line, ri.args[0], err)
		}
		compiledRegexes = append(compiledRegexes, compiledRegex{re: re, inst: ri})
		for _, name := range re.SubexpNames() {
			if name != "" && !colSet[name] {
				columns = append(columns, name)
				colSet[name] = true
			}
		}
	}
	// Also include SET columns
	for _, si := range setInsts {
		if len(si.args) >= 1 {
			name := strings.TrimPrefix(si.args[0], "$")
			if !colSet[name] {
				columns = append(columns, name)
				colSet[name] = true
			}
		}
	}

	// Phase 2+3: For each data line, apply REPLACE then all REGEXes
	var rows []map[string]string
	for _, line := range dataLines {
		processed := line
		for _, ri := range replaceInsts {
			processed, err = applyReplace(ri, processed)
			if err != nil {
				return PipelineResult{}, err
			}
		}
		if strings.TrimSpace(processed) == "" {
			continue
		}

		row := make(map[string]string)
		matched := false
		for _, cr := range compiledRegexes {
			m := cr.re.FindStringSubmatch(processed)
			if m == nil {
				continue
			}
			matched = true
			for i, name := range cr.re.SubexpNames() {
				if i == 0 || name == "" {
					continue
				}
				row[name] = m[i]
			}
		}
		if !matched {
			continue
		}

		for _, si := range setInsts {
			if err := applySet(si, row); err != nil {
				return PipelineResult{}, err
			}
		}

		rows = append(rows, row)
	}

	return PipelineResult{
		Rows:    rows,
		Columns: columns,
		Mode:    "table",
	}, nil
}

// ── Trimming Engine ──────────────────────────────────────────────────────

func applyTrimming(insts []instruction, lines []string) ([]string, error) {
	// Process SKIP_UNTIL, SKIP_LINES, SKIP_BLANK first (order-dependent)
	// Then apply FILTER and REJECT on remaining lines
	// STOP_AT terminates processing

	type trimState struct {
		started  bool // after SKIP_UNTIL is satisfied
		skipN    int  // remaining lines to skip (from SKIP_LINES)
	}

	// Collect pre-compiled patterns
	var skipUntilRe *regexp.Regexp
	var stopAtRe *regexp.Regexp
	var filterRe *regexp.Regexp
	var rejectRe *regexp.Regexp
	skipBlank := false
	skipLinesN := 0

	// Build the trim pipeline from instructions (in order)
	// Note: we process them in sequence to handle dependencies
	type trimOp struct {
		kind    string // "skip_until", "skip_lines", "skip_blank", "stop_at", "filter", "reject"
		re      *regexp.Regexp
		n       int
	}
	var ops []trimOp

	for _, inst := range insts {
		switch inst.verb {
		case "SKIP_UNTIL":
			if len(inst.args) == 0 {
				return nil, fmt.Errorf("pipeline line %d: SKIP_UNTIL requires a regex pattern", inst.line)
			}
			re, err := regexp.Compile(inst.args[0])
			if err != nil {
				return nil, fmt.Errorf("pipeline line %d: invalid regex %q: %w", inst.line, inst.args[0], err)
			}
			skipUntilRe = re
			ops = append(ops, trimOp{kind: "skip_until", re: re})

		case "SKIP_LINES":
			if len(inst.args) == 0 {
				return nil, fmt.Errorf("pipeline line %d: SKIP_LINES requires a number", inst.line)
			}
			n := 0
			fmt.Sscanf(inst.args[0], "%d", &n)
			skipLinesN = n
			ops = append(ops, trimOp{kind: "skip_lines", n: n})

		case "SKIP_BLANK":
			skipBlank = true
			ops = append(ops, trimOp{kind: "skip_blank"})

		case "STOP_AT":
			if len(inst.args) == 0 {
				return nil, fmt.Errorf("pipeline line %d: STOP_AT requires a regex pattern", inst.line)
			}
			re, err := regexp.Compile(inst.args[0])
			if err != nil {
				return nil, fmt.Errorf("pipeline line %d: invalid regex %q: %w", inst.line, inst.args[0], err)
			}
			stopAtRe = re
			ops = append(ops, trimOp{kind: "stop_at", re: re})

		case "FILTER":
			if len(inst.args) == 0 {
				return nil, fmt.Errorf("pipeline line %d: FILTER requires a regex pattern", inst.line)
			}
			re, err := regexp.Compile(inst.args[0])
			if err != nil {
				return nil, fmt.Errorf("pipeline line %d: invalid regex %q: %w", inst.line, inst.args[0], err)
			}
			filterRe = re
			ops = append(ops, trimOp{kind: "filter", re: re})

		case "REJECT":
			if len(inst.args) == 0 {
				return nil, fmt.Errorf("pipeline line %d: REJECT requires a regex pattern", inst.line)
			}
			re, err := regexp.Compile(inst.args[0])
			if err != nil {
				return nil, fmt.Errorf("pipeline line %d: invalid regex %q: %w", inst.line, inst.args[0], err)
			}
			rejectRe = re
			ops = append(ops, trimOp{kind: "reject", re: re})
		}
	}

	// Execute the trimming pipeline
	_ = skipBlank
	_ = skipLinesN
	_ = skipUntilRe

	var result []string
	started := skipUntilRe == nil // if no SKIP_UNTIL, start immediately
	skipRemaining := 0

	for _, line := range lines {
		// STOP_AT: stop processing
		if stopAtRe != nil && started && stopAtRe.MatchString(line) {
			break
		}

		// SKIP_UNTIL: wait for matching line
		if !started {
			if skipUntilRe.MatchString(line) {
				started = true
				// The matching line itself is skipped (it's the header)
				skipRemaining = skipLinesN
				continue
			}
			continue
		}

		// SKIP_LINES: skip N lines after SKIP_UNTIL
		if skipRemaining > 0 {
			skipRemaining--
			continue
		}

		// SKIP_BLANK
		if skipBlank && strings.TrimSpace(line) == "" {
			continue
		}

		// FILTER: only keep matching lines
		if filterRe != nil && !filterRe.MatchString(line) {
			continue
		}

		// REJECT: discard matching lines
		if rejectRe != nil && rejectRe.MatchString(line) {
			continue
		}

		result = append(result, line)
	}
	return result, nil
}

// ── SPLIT ────────────────────────────────────────────────────────────────

func applySplit(columns []string, line string) map[string]string {
	fields := strings.Fields(line)
	row := make(map[string]string, len(columns))

	for i, col := range columns {
		if i == len(columns)-1 {
			// Last variable gets all remaining fields joined by space
			if i < len(fields) {
				row[col] = strings.Join(fields[i:], " ")
			} else {
				row[col] = ""
			}
		} else {
			if i < len(fields) {
				row[col] = fields[i]
			} else {
				row[col] = ""
			}
		}
	}
	return row
}

// ── REPLACE ──────────────────────────────────────────────────────────────

func applyReplace(inst instruction, text string) (string, error) {
	if len(inst.args) < 2 {
		return "", fmt.Errorf("pipeline line %d: REPLACE requires <pattern> <replacement>", inst.line)
	}
	pattern := inst.args[0]
	replacement := inst.args[1]

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("pipeline line %d: invalid regex %q: %w", inst.line, pattern, err)
	}
	return re.ReplaceAllString(text, replacement), nil
}

// ── SET ──────────────────────────────────────────────────────────────────

// applySet handles:
//   SET $name <literal>
//   SET $name $other
//   SET $name $a "/" $b                          (concatenation)
//   SET $name ($left == "val" ? "yes" : "no")    (simple comparison ternary)
//   SET $name (REGEX_MATCH($var, "pat") ? "y" : "n")  (regex ternary)
//
// To avoid issues with token-based parsing (which breaks function calls with
// commas and parentheses), we re-extract the full expression from inst.raw.
func applySet(inst instruction, row map[string]string) error {
	if len(inst.args) < 2 {
		return fmt.Errorf("pipeline line %d: SET requires $name <expr>", inst.line)
	}
	varName := strings.TrimPrefix(inst.args[0], "$")

	// Re-extract the full expression from the raw line to preserve structure.
	// raw line looks like: "SET $varName <expr>"
	rawTrimmed := strings.TrimSpace(inst.raw)
	// Skip "SET"
	_, afterVerb := splitFirst(rawTrimmed)
	// Skip "$varName"
	_, expr := splitFirst(afterVerb)
	expr = strings.TrimSpace(expr)

	// Strip optional outer parentheses: (expr) → expr
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}

	// ── Try ternary: <condition> ? <trueExpr> : <falseExpr> ──
	// Find the top-level "?" that is NOT inside parentheses or quotes
	qIdx := findTopLevel(expr, '?')
	if qIdx > 0 {
		condStr := strings.TrimSpace(expr[:qIdx])
		rest := strings.TrimSpace(expr[qIdx+1:])

		// Find top-level ":" in the rest
		cIdx := findTopLevel(rest, ':')
		if cIdx < 0 {
			return fmt.Errorf("pipeline line %d: ternary missing ':' in SET", inst.line)
		}
		trueStr := strings.TrimSpace(rest[:cIdx])
		falseStr := strings.TrimSpace(rest[cIdx+1:])

		condResult, err := evalCondition(condStr, row, inst.line)
		if err != nil {
			return err
		}
		if condResult {
			row[varName] = evalValue(trueStr, row)
		} else {
			row[varName] = evalValue(falseStr, row)
		}
		return nil
	}

	// ── Not a ternary — concatenation or simple assignment ──
	// Fall back to token-based approach for concatenation
	exprParts := inst.args[1:]
	var parts []string
	for _, p := range exprParts {
		parts = append(parts, resolveVar(p, row))
	}
	row[varName] = strings.Join(parts, "")
	return nil
}

// findTopLevel finds the index of char c in s that is NOT inside parentheses or quotes.
// Returns -1 if not found.
func findTopLevel(s string, c byte) int {
	depth := 0
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == '\\' && i+1 < len(s) {
				i++ // skip escaped char
				continue
			}
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			continue
		}
		if ch == c && depth == 0 {
			return i
		}
	}
	return -1
}

// evalCondition evaluates a condition expression, which can be:
//   - $a == "val"         (simple comparison)
//   - $a != "val"         (not equal)
//   - REGEX_MATCH($var, "pattern")  (regex match, returns true/false)
func evalCondition(cond string, row map[string]string, lineNum int) (bool, error) {
	cond = strings.TrimSpace(cond)

	// Check for REGEX_MATCH function
	if strings.HasPrefix(cond, "REGEX_MATCH(") && strings.HasSuffix(cond, ")") {
		inner := cond[len("REGEX_MATCH(") : len(cond)-1]
		// Split by comma: $var, "pattern"
		commaIdx := strings.Index(inner, ",")
		if commaIdx < 0 {
			return false, fmt.Errorf("pipeline line %d: REGEX_MATCH requires ($var, \"pattern\")", lineNum)
		}
		varRef := strings.TrimSpace(inner[:commaIdx])
		patternStr := strings.TrimSpace(inner[commaIdx+1:])

		varVal := resolveVar(varRef, row)
		pattern := unquote(patternStr)

		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("pipeline line %d: invalid regex in REGEX_MATCH %q: %w", lineNum, pattern, err)
		}
		return re.MatchString(varVal), nil
	}

	// Simple comparison: <left> == <right>  or  <left> != <right>
	for _, op := range []string{"==", "!="} {
		idx := strings.Index(cond, " "+op+" ")
		if idx < 0 {
			continue
		}
		left := resolveVar(strings.TrimSpace(cond[:idx]), row)
		right := resolveVar(strings.TrimSpace(cond[idx+len(op)+2:]), row)
		// Unquote if quoted
		left = unquote(left)
		right = unquote(right)
		switch op {
		case "==":
			return left == right, nil
		case "!=":
			return left != right, nil
		}
	}

	return false, fmt.Errorf("pipeline line %d: unsupported condition %q in SET", lineNum, cond)
}

// evalValue evaluates a value expression (for ternary true/false branches).
// Handles $var references and quoted literals.
func evalValue(s string, row map[string]string) string {
	s = strings.TrimSpace(s)
	return resolveVar(unquote(s), row)
}

// unquote removes surrounding double or single quotes if present,
// and processes common escape sequences (\\→\, \n→newline, etc.).
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
			// Process escape sequences inside quoted strings
			s = processEscapes(s)
		}
	}
	return s
}

// processEscapes handles common escape sequences in quoted strings.
func processEscapes(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\\':
				b.WriteByte('\\')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case '"':
				b.WriteByte('"')
				i++
			case '\'':
				b.WriteByte('\'')
				i++
			default:
				// Keep the backslash as-is for regex patterns like \s, \d, etc.
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// resolveVar resolves a value: $name → row[name], literal → literal.
func resolveVar(s string, row map[string]string) string {
	s = unquote(s)
	if strings.HasPrefix(s, "$") {
		name := strings.TrimPrefix(s, "$")
		return row[name]
	}
	return s
}
