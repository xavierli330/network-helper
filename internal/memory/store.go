package memory

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/xavierli/nethelper/internal/store"
)

// Entry is a single memory record stored in the database.
type Entry struct {
	ID        int
	Category  string // "conversation", "insight", "preference"
	Content   string
	Embedding []float32
	CreatedAt time.Time
	SessionID string
}

// Insert stores a memory entry with its embedding vector.
func Insert(db *store.DB, category, content, sessionID string, embedding []float32) error {
	blob := float32sToBytes(embedding)
	_, err := db.Exec(
		`INSERT INTO memory_entries (category, content, embedding, session_id) VALUES (?, ?, ?, ?)`,
		category, content, blob, sessionID,
	)
	return err
}

// ListAll returns all memory entries with embeddings for search.
func ListAll(db *store.DB) ([]Entry, error) {
	rows, err := db.Query(
		`SELECT id, category, content, embedding, created_at, session_id FROM memory_entries ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var blob []byte
		if err := rows.Scan(&e.ID, &e.Category, &e.Content, &blob, &e.CreatedAt, &e.SessionID); err != nil {
			return nil, err
		}
		e.Embedding = bytesToFloat32s(blob)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Search finds the top-K most similar memories to the query vector.
// Only returns entries with cosine similarity > 0.3.
func Search(db *store.DB, queryVec []float32, topK int) ([]Entry, error) {
	all, err := ListAll(db)
	if err != nil {
		return nil, err
	}

	type scored struct {
		entry Entry
		score float64
	}
	var results []scored
	for _, e := range all {
		if len(e.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, e.Embedding)
		results = append(results, scored{e, sim})
	}

	// Sort by score descending (simple selection sort — memory list is small)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	const minSimilarity = 0.3
	var top []Entry
	for i := 0; i < topK && i < len(results); i++ {
		if results[i].score > minSimilarity {
			top = append(top, results[i].entry)
		}
	}
	return top, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func float32sToBytes(f []float32) []byte {
	buf := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func bytesToFloat32s(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	f := make([]float32, len(b)/4)
	for i := range f {
		f[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return f
}
