package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/ghostlawless/xdl/internal/config"
	"github.com/ghostlawless/xdl/internal/httpx"
	"github.com/ghostlawless/xdl/internal/log"
	xruntime "github.com/ghostlawless/xdl/internal/runtime"
	"github.com/ghostlawless/xdl/internal/utils"
)

type Media struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

func GetMediaLinksForUser(cl *http.Client, cf *config.EssentialsConfig, uid string, sn string, vb bool, lim *xruntime.Limiter) ([]Media, error) {
	if cl == nil || cf == nil {
		return nil, errors.New("nil client or config")
	}
	if uid == "" {
		return nil, errors.New("empty userID")
	}

	ep, err := cf.GraphQLURL("user_media")
	if err != nil {
		return nil, err
	}

	allm := make(map[string]Media, 512)
	cur := ""
	pg := 1
	stg := 0
	const mx = 200
	seen := make(map[string]struct{}, 256)
	seen[""] = struct{}{}

	end := ""
	ic := 0
	vc := 0
	last := 0
	ri := 0
	ref := strings.TrimRight(cf.X.Network, "/") + "/i/user/" + uid + "/media"
	frames := []rune{'|', '/', '-', '\\'}

	for {
		ri++
		if lim != nil {
			lim.SleepBeforeRequest(context.Background(), sn, pg, ri)
		}

		vars := map[string]any{
			"userId":                 uid,
			"count":                  100,
			"includePromotedContent": false,
			"withClientEventToken":   false,
			"withVoice":              false,
		}
		if cur != "" {
			vars["cursor"] = cur
		}
		vj, _ := json.Marshal(vars)
		fj, _ := cf.FeatureJSONFor("user_media")
		q := fmt.Sprintf("%s?variables=%s&features=%s", ep, url.QueryEscape(string(vj)), url.QueryEscape(fj))

		rq, gerr := http.NewRequest(http.MethodGet, q, nil)
		if gerr != nil {
			return nil, fmt.Errorf("build request: %w", gerr)
		}
		cf.BuildRequestHeaders(rq, ref)
		rq.Header.Set("Accept", "application/json, */*;q=0.1")

		prev := len(allm)
		b, st, err := httpx.DoRequestWithOptions(cl, rq, httpx.RequestOptions{
			MaxBytes: 8 << 20,
			Decode:   true,
			Accept:   func(s int) bool { return s >= 200 && s < 300 },
		})
		if err != nil {
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_user_media", "json", b)
				meta := fmt.Sprintf("METHOD: GET\nSTATUS: %d\nURL: %s\nPAGE: %d\nCURSOR: %s\n", st, q, pg, cur)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). see: %s", st, p))
			} else {
				log.LogError("media", fmt.Sprintf("UserMedia failed (status %d). run with -d for details.", st))
			}
			end = "http_error"
			break
		}

		pms, jerr := fold(b)
		if jerr != nil {
			if cf.Runtime.DebugEnabled {
				p, _ := utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_parse", "json", b)
				meta := fmt.Sprintf("PARSE_ERROR: %v\nPAGE: %d\nCURSOR: %s\n", jerr, pg, cur)
				_, _ = utils.SaveTimestamped(cf.Paths.Debug, "err_user_media_parse_meta", "txt", []byte(meta))
				log.LogError("media", fmt.Sprintf("parse page %d failed. see: %s", pg, p))
			} else {
				log.LogError("media", fmt.Sprintf("parse page %d failed.", pg))
			}
			end = "parse_error"
			break
		}

		for _, m := range pms {
			if _, ok := allm[m.URL]; !ok {
				allm[m.URL] = m
				if m.Type == "image" {
					ic++
				} else if m.Type == "video" {
					vc++
				}
			}
		}

		now := len(allm)
		if now == prev {
			stg++
		} else {
			stg = 0
		}

		if cf.Runtime.DebugEnabled {
			d := now - prev
			log.LogInfo("media", fmt.Sprintf("page %d: +%d (total %d)", pg, d, now))
		}

		if vb {
			t := len(allm)
			fr := frames[(pg-1)%len(frames)]
			line := fmt.Sprintf("xdl ▸ scanning media for target @%s [%c] — page:%d images:%d videos:%d (total:%d)", sn, fr, pg, ic, vc, t)
			if len(line) < last {
				line += strings.Repeat(" ", last-len(line))
			}
			last = len(line)
			fmt.Printf("\r\033[2K%s", line)
		}

		if stg >= 3 {
			log.LogInfo("media", "no progress for 3 pages — stopping")
			end = "no_progress"
			break
		}

		nx := next(b)
		if nx == "" {
			log.LogInfo("media", "no next cursor — reached end of timeline")
			end = "no_next_cursor"
			break
		}
		if _, dup := seen[nx]; dup {
			log.LogInfo("media", "repeated cursor detected — stopping")
			end = "repeat_cursor"
			break
		}
		seen[nx] = struct{}{}

		if pg >= mx {
			log.LogInfo("media", fmt.Sprintf("max pages reached (%d) — stopping", mx))
			end = "max_pages"
			break
		}

		cur = nx
		pg++
	}

	if vb && last > 0 {
		fmt.Print("\n")
	}

	keys := make([]string, 0, len(allm))
	for k := range allm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Media, 0, len(keys))
	for _, k := range keys {
		out = append(out, allm[k])
	}

	log.LogInfo("media", fmt.Sprintf("Total unique media found: %d", len(out)))

	if end == "no_progress" || end == "no_next_cursor" || end == "repeat_cursor" || end == "max_pages" {
		log.LogInfo("media", fmt.Sprintf("UserMedia endpoint reached its server-side end at page %d. This feed may expose fewer items than the media counter shown in the profile UI.", pg))
	}

	return out, nil
}

func fold(b []byte) ([]Media, error) {
	var r any
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	ag := make(map[string]mAgg, 256)
	walk(r, ag)
	out := make([]Media, 0, len(ag))
	for _, v := range ag {
		out = append(out, Media{URL: v.URL, Type: v.Type})
	}
	return out, nil
}

type mAgg struct {
	URL     string
	Type    string
	Bitrate int
}

func walk(v any, ag map[string]mAgg) {
	switch t := v.(type) {
	case map[string]any:
		if leg, ok := t["legacy"].(map[string]any); ok {
			gather(leg, ag)
		}
		if _, ok := t["extended_entities"]; ok {
			gather(t, ag)
		}
		for _, vv := range t {
			walk(vv, ag)
		}
	case []any:
		for _, it := range t {
			walk(it, ag)
		}
	}
}

func gather(n map[string]any, ag map[string]mAgg) {
	ee, ok := n["extended_entities"].(map[string]any)
	if !ok {
		return
	}
	arr, ok := ee["media"].([]any)
	if !ok {
		return
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(str(m["type"]))
		id := mid(m)
		if id == "" {
			id = midFrom(str(m["media_url_https"]))
		}
		if id == "" {
			continue
		}
		switch typ {
		case "photo":
			u := norm(str(m["media_url_https"]))
			if u == "" {
				continue
			}
			ag[id] = mAgg{URL: u, Type: "image"}
		case "video", "animated_gif":
			u, br := best(m)
			if u == "" {
				continue
			}
			if prev, ok := ag[id]; !ok || br > prev.Bitrate {
				ag[id] = mAgg{URL: u, Type: "video", Bitrate: br}
			}
		}
	}
}

func mid(m map[string]any) string {
	if s := str(m["id_str"]); s != "" {
		return s
	}
	if s := str(m["media_key"]); s != "" {
		return s
	}
	if f, ok := m["id"].(float64); ok && f > 0 {
		return strconv.FormatInt(int64(f), 10)
	}
	return ""
}

func norm(u string) string {
	if u == "" {
		return ""
	}
	pu, err := url.Parse(u)
	if err != nil {
		return u
	}
	if !strings.Contains(strings.ToLower(pu.Host), "twimg.com") {
		return u
	}
	q := pu.Query()
	format := q.Get("format")
	q = url.Values{}
	if format != "" {
		q.Set("format", format)
	}
	q.Set("name", "orig")
	pu.RawQuery = q.Encode()
	return pu.String()
}

func best(m map[string]any) (string, int) {
	vi, ok := m["video_info"].(map[string]any)
	if !ok {
		return "", 0
	}
	vs, ok := vi["variants"].([]any)
	if !ok || len(vs) == 0 {
		return "", 0
	}
	u := ""
	br := -1
	for _, it := range vs {
		mv, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ct := strings.ToLower(str(mv["content_type"]))
		if !strings.Contains(ct, "video/mp4") {
			continue
		}
		x := str(mv["url"])
		if x == "" {
			continue
		}
		b := 0
		if f, ok := mv["bitrate"].(float64); ok {
			b = int(f)
		}
		if b > br {
			br = b
			u = x
		}
	}
	if u == "" {
		return "", 0
	}
	return u, br
}

func midFrom(u string) string {
	if u == "" {
		return ""
	}
	pu, err := url.Parse(u)
	if err != nil {
		return ""
	}
	base := path.Base(pu.Path)
	base = strings.SplitN(base, ".", 2)[0]
	base = strings.TrimSpace(base)
	return base
}

func next(b []byte) string {
	var r any
	if err := json.Unmarshal(b, &r); err != nil {
		return ""
	}
	if v := bottom(r); v != "" {
		return v
	}
	return anyc(r)
}

func bottom(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if strings.EqualFold(str(t["cursorType"]), "Bottom") {
			if val := str(t["value"]); val != "" {
				return val
			}
		}
		for _, vv := range t {
			if got := bottom(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := bottom(it); got != "" {
				return got
			}
		}
	}
	return ""
}

func anyc(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if strings.Contains(strings.ToLower(k), "cursor") {
				if s := str(vv); s != "" {
					return s
				}
			}
		}
		for _, vv := range t {
			if got := anyc(vv); got != "" {
				return got
			}
		}
	case []any:
		for _, it := range t {
			if got := anyc(it); got != "" {
				return got
			}
		}
	}
	return ""
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
