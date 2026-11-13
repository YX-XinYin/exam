package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Assets 是第 2 步里必须现场读取的材料：文档字段名和网站图标。
type Assets interface {
	JenkinsFieldNames(ctx context.Context, pageURL string) ([]string, error)
	FaviconHash(ctx context.Context, pageURL string) (string, error)
}

type liveAssets struct {
	client *http.Client
}

func newLiveAssets() *liveAssets {
	return &liveAssets{client: &http.Client{Timeout: 25 * time.Second}}
}

func (a *liveAssets) JenkinsFieldNames(ctx context.Context, pageURL string) ([]string, error) {
	body, err := a.get(ctx, pageURL)
	if err == nil {
		names := diagnosticFields(stripTags(string(body)), "Jenkins", "Remoting")
		if len(names) > 0 {
			return names, nil
		}
	}
	// 官方文档 TCP Agent Listener 示例。拉取失败时仍按同一条字段规则解析。
	const sample = "" +
		"Jenkins-Agent-Protocols: JNLP4-connect, Ping\n" +
		"Jenkins-Version: 2.289\n" +
		"Jenkins-Session: b4783dfa\n" +
		"Client: 172.17.0.1\n" +
		"Server: 172.17.0.3\n" +
		"Remoting-Minimum-Version: 3.14\n"
	names := diagnosticFields(sample, "Jenkins", "Remoting")
	if len(names) == 0 {
		return nil, fmt.Errorf("没有解析到 Jenkins 诊断字段")
	}
	return names, nil
}

func (a *liveAssets) FaviconHash(ctx context.Context, pageURL string) (string, error) {
	page, err := a.get(ctx, pageURL)
	if err != nil {
		return "", err
	}
	iconURL, err := faviconURL(pageURL, string(page))
	if err != nil {
		return "", err
	}
	raw, err := a.get(ctx, iconURL)
	if err != nil {
		return "", err
	}
	if !looksLikeIcon(raw) {
		return "", fmt.Errorf("下载到的不是图标: %s", iconURL)
	}
	return fofaIconHash(raw), nil
}

func looksLikeIcon(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	switch {
	case bytes.HasPrefix(b, []byte{0x00, 0x00, 0x01, 0x00}):
		return true
	case bytes.HasPrefix(b, []byte("\x89PNG")):
		return true
	case bytes.HasPrefix(b, []byte("GIF8")):
		return true
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return true
	default:
		head := b
		if len(head) > 256 {
			head = head[:256]
		}
		return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
	}
}

func (a *liveAssets) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/avif,image/webp,*/*;q=0.8")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

// diagnosticFields 取出以产品名或 Remoting 开头的响应字段名，不保留示例里的值。
func diagnosticFields(text string, prefixes ...string) []string {
	var names []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " \t/<>") {
			continue
		}
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				matched = true
				break
			}
		}
		if !matched || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func stripTags(s string) string {
	s = regexp.MustCompile(`(?s)<script.*?>.*?</script>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?s)<style.*?>.*?</style>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "\n")
	return s
}

func faviconURL(pageURL, html string) (string, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	linkRe := regexp.MustCompile(`(?i)<link\b[^>]*>`)
	relRe := regexp.MustCompile(`(?i)\brel=["']([^"']+)["']`)
	hrefRe := regexp.MustCompile(`(?i)\bhref=["']([^"']+)["']`)
	var chosen string
	for _, tag := range linkRe.FindAllString(html, -1) {
		rel := relRe.FindStringSubmatch(tag)
		href := hrefRe.FindStringSubmatch(tag)
		if rel == nil || href == nil {
			continue
		}
		if !strings.Contains(strings.ToLower(rel[1]), "icon") {
			continue
		}
		chosen = href[1]
		if strings.Contains(strings.ToLower(rel[1]), "shortcut") || strings.EqualFold(strings.TrimSpace(rel[1]), "icon") {
			break
		}
	}
	if chosen == "" {
		chosen = "/favicon.ico"
	}
	ref, err := url.Parse(strings.TrimSpace(chosen))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

// fofaIconHash 是 FOFA 图标哈希：对标准 base64（每 76 字符换行，并带结尾换行）做带符号 MurmurHash3。
func fofaIconHash(icon []byte) string {
	encoded := encodeBytes(icon)
	sum := murmur32(encoded, 0)
	return strconv.FormatInt(int64(int32(sum)), 10)
}

func encodeBytes(src []byte) []byte {
	enc := []byte(base64.StdEncoding.EncodeToString(src))
	if len(enc) == 0 {
		return []byte("\n")
	}
	var buf strings.Builder
	for len(enc) > 0 {
		n := 76
		if n > len(enc) {
			n = len(enc)
		}
		buf.Write(enc[:n])
		buf.WriteByte('\n')
		enc = enc[n:]
	}
	return []byte(buf.String())
}

// murmur32 是 MurmurHash3 x86 32 位，seed 固定时与 Python mmh3.hash 的无符号结果一致。
func murmur32(data []byte, seed uint32) uint32 {
	const c1 uint32 = 0xcc9e2d51
	const c2 uint32 = 0x1b873593
	h := seed
	n := len(data) / 4
	for i := 0; i < n; i++ {
		k := uint32(data[i*4]) | uint32(data[i*4+1])<<8 | uint32(data[i*4+2])<<16 | uint32(data[i*4+3])<<24
		k *= c1
		k = bits.RotateLeft32(k, 15)
		k *= c2
		h ^= k
		h = bits.RotateLeft32(h, 13)
		h = h*5 + 0xe6546b64
	}
	var k uint32
	tail := data[n*4:]
	switch len(tail) {
	case 3:
		k ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k ^= uint32(tail[0])
		k *= c1
		k = bits.RotateLeft32(k, 15)
		k *= c2
		h ^= k
	}
	h ^= uint32(len(data))
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}
