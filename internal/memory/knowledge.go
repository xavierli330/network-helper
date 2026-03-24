package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/store"
)

// KnowledgeEntry is a chunk of external knowledge with its embedding.
type KnowledgeEntry struct {
	Source    string // base file name
	Content   string // text chunk
	Embedding []float32
}

// KnowledgeBase holds embedded knowledge loaded from local .md files.
type KnowledgeBase struct {
	entries []KnowledgeEntry
}

// LoadKnowledge reads all .md files from knowledgeDir, chunks them,
// embeds each chunk, and caches the embeddings in the DB.
// Returns nil if knowledgeDir doesn't exist, has no .md files, or embedder is nil.
func LoadKnowledge(ctx context.Context, knowledgeDir string, embedder llm.Embedder, db *store.DB) *KnowledgeBase {
	if embedder == nil {
		return nil
	}

	files, err := filepath.Glob(filepath.Join(knowledgeDir, "*.md"))
	if err != nil || len(files) == 0 {
		return nil
	}

	kb := &KnowledgeBase{}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Printf("[knowledge] read error for %s: %v", f, err)
			continue
		}

		// Compute content hash for cache invalidation.
		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		// Try to load cached embeddings from DB.
		cached := loadCachedKnowledge(db, f, hashStr)
		if cached != nil {
			kb.entries = append(kb.entries, cached...)
			log.Printf("[knowledge] cache hit for %s (%d chunks)", filepath.Base(f), len(cached))
			continue
		}

		content := string(data)
		chunks := chunkDocument(content, filepath.Base(f))

		var fileEntries []KnowledgeEntry
		for _, chunk := range chunks {
			embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			vec, err := embedder.Embed(embedCtx, chunk)
			cancel()
			if err != nil {
				log.Printf("[knowledge] embed error for %s: %v", f, err)
				continue
			}
			entry := KnowledgeEntry{
				Source:    filepath.Base(f),
				Content:   chunk,
				Embedding: vec,
			}
			fileEntries = append(fileEntries, entry)
			cacheKnowledge(db, f, hashStr, chunk, vec)
		}

		kb.entries = append(kb.entries, fileEntries...)
		log.Printf("[knowledge] loaded %s (%d chunks)", filepath.Base(f), len(fileEntries))
	}

	if len(kb.entries) == 0 {
		return nil
	}
	return kb
}

// Search finds the most relevant knowledge chunks for a query vector.
// Only entries with cosine similarity > 0.3 are returned.
func (kb *KnowledgeBase) Search(queryVec []float32, topK int) []KnowledgeEntry {
	if kb == nil || len(kb.entries) == 0 {
		return nil
	}

	type scored struct {
		entry KnowledgeEntry
		score float64
	}
	var results []scored
	for _, e := range kb.entries {
		if len(e.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, e.Embedding)
		results = append(results, scored{e, sim})
	}

	// Sort descending by score (selection sort — list is typically small).
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	const minSimilarity = 0.3
	var top []KnowledgeEntry
	for i := 0; i < topK && i < len(results); i++ {
		if results[i].score > minSimilarity {
			top = append(top, results[i].entry)
		}
	}
	return top
}

// chunkDocument splits a markdown document into chunks by ## headers.
// If no ## headers are found, it falls back to ~1000-character boundaries.
func chunkDocument(content, _ string) []string {
	// Split on "\n## " to catch second-level headers.
	sections := strings.Split(content, "\n## ")
	if len(sections) > 1 {
		var chunks []string
		for i, s := range sections {
			if i > 0 {
				s = "## " + s
			}
			s = strings.TrimSpace(s)
			if len(s) > 50 {
				chunks = append(chunks, s)
			}
		}
		if len(chunks) > 0 {
			return chunks
		}
	}

	// No headers — chunk at ~1000-char boundaries on newlines.
	const chunkSize = 1000
	var chunks []string
	for len(content) > 0 {
		end := chunkSize
		if end > len(content) {
			end = len(content)
		}
		// Prefer breaking at a newline after the halfway point.
		if end < len(content) {
			if idx := strings.LastIndex(content[:end], "\n"); idx > chunkSize/2 {
				end = idx + 1
			}
		}
		chunk := strings.TrimSpace(content[:end])
		if len(chunk) > 50 {
			chunks = append(chunks, chunk)
		}
		content = content[end:]
	}
	return chunks
}

// loadCachedKnowledge retrieves knowledge chunks from the DB for the given
// file path and content hash. Returns nil when no cached rows are found.
func loadCachedKnowledge(db *store.DB, filePath, hash string) []KnowledgeEntry {
	if db == nil {
		return nil
	}
	rows, err := db.Query(
		`SELECT content, embedding FROM knowledge_cache WHERE file_path = ? AND file_hash = ?`,
		filePath, hash,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []KnowledgeEntry
	for rows.Next() {
		var content string
		var blob []byte
		if rows.Scan(&content, &blob) != nil {
			continue
		}
		entries = append(entries, KnowledgeEntry{
			Source:    filepath.Base(filePath),
			Content:   content,
			Embedding: bytesToFloat32s(blob),
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// cacheKnowledge persists a single knowledge chunk and its embedding vector.
func cacheKnowledge(db *store.DB, filePath, hash, content string, vec []float32) {
	if db == nil {
		return
	}
	db.Exec(
		`INSERT INTO knowledge_cache (file_path, file_hash, content, embedding) VALUES (?, ?, ?, ?)`,
		filePath, hash, content, float32sToBytes(vec),
	)
}
