package downloader

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ghostlawless/xdl/internal/scraper"
	"github.com/ghostlawless/xdl/internal/utils"
)

type CheckpointStatus string

const (
	CheckpointPending CheckpointStatus = "pending"
	CheckpointDone    CheckpointStatus = "done"
	CheckpointSkipped CheckpointStatus = "skipped"
	CheckpointFailed  CheckpointStatus = "failed"
)

const checkpointVersion = 1

type CheckpointItem struct {
	Index  int              `json:"index"`
	URL    string           `json:"url"`
	Type   string           `json:"type"`
	Status CheckpointStatus `json:"status"`
	Size   int64            `json:"size"`
}

type Checkpoint struct {
	Version   int              `json:"version"`
	User      string           `json:"user"`
	RunID     string           `json:"run_id"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Items     []CheckpointItem `json:"items"`
	urlIndex  map[string]int   `json:"-"`
}

func NewCheckpoint(user, runID string, medias []scraper.Media) *Checkpoint {
	t := time.Now().UTC()
	xs := make([]CheckpointItem, len(medias))
	for i, m := range medias {
		xs[i] = CheckpointItem{Index: i, URL: m.URL, Type: m.Type, Status: CheckpointPending, Size: 0}
	}
	cp := &Checkpoint{
		Version:   checkpointVersion,
		User:      user,
		RunID:     runID,
		CreatedAt: t,
		UpdatedAt: t,
		Items:     xs,
	}
	cp.ix()
	return cp
}

func (c *Checkpoint) ix() {
	c.urlIndex = make(map[string]int, len(c.Items))
	for i, it := range c.Items {
		if it.URL != "" {
			c.urlIndex[it.URL] = i
		}
	}
}

func (c *Checkpoint) touch() {
	c.UpdatedAt = time.Now().UTC()
}

func (c *Checkpoint) MarkByIndex(idx int, s CheckpointStatus, sz int64) {
	if c == nil {
		return
	}
	if idx < 0 || idx >= len(c.Items) {
		return
	}
	it := c.Items[idx]
	it.Status = s
	if sz >= 0 {
		it.Size = sz
	}
	c.Items[idx] = it
	c.touch()
}

func (c *Checkpoint) MarkByURL(u string, s CheckpointStatus, sz int64) {
	if c == nil || u == "" {
		return
	}
	if c.urlIndex == nil {
		c.ix()
	}
	i, ok := c.urlIndex[u]
	if !ok {
		return
	}
	c.MarkByIndex(i, s, sz)
}

func (c *Checkpoint) PendingItems() []CheckpointItem {
	if c == nil {
		return nil
	}
	out := make([]CheckpointItem, 0, len(c.Items))
	for _, it := range c.Items {
		if it.Status == CheckpointPending {
			out = append(out, it)
		}
	}
	return out
}

func (c *Checkpoint) CompletedCount() (done, skipped, failed int) {
	if c == nil {
		return
	}
	for _, it := range c.Items {
		switch it.Status {
		case CheckpointDone:
			done++
		case CheckpointSkipped:
			skipped++
		case CheckpointFailed:
			failed++
		}
	}
	return
}

func (c *Checkpoint) Save(p string) error {
	if c == nil {
		return errors.New("nil checkpoint")
	}
	if p == "" {
		return errors.New("empty checkpoint path")
	}
	c.touch()
	d := filepath.Dir(p)
	if err := utils.EnsureDir(d); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return utils.SaveToFile(p, b)
}

func LoadCheckpoint(p string) (*Checkpoint, error) {
	if p == "" {
		return nil, errors.New("empty checkpoint path")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	if cp.Version <= 0 {
		cp.Version = checkpointVersion
	}
	cp.ix()
	return &cp, nil
}
