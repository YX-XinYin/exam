package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// 本参赛包 README 给出的编号。三个来源必须一致。
const expectedPackageID = "pkg-08e82c8d"

type Question struct {
	ID   string `json:"题号"`
	Text string `json:"自然语言输入"`
}

type answerFile struct {
	Name      string   `json:"选手名称"`
	PackageID string   `json:"参赛包编号"`
	Answers   []Answer `json:"答案"`
}

type Answer struct {
	ID    string `json:"题号"`
	Query string `json:"查询语句"`
}

type Bundle struct {
	PackageID string
	Issued    string
	Questions []Question
}

func LoadBundle(questionPath, templatePath, packPath string) (*Bundle, error) {
	qText, err := docxText(questionPath)
	if err != nil {
		return nil, fmt.Errorf("读取题目: %w", err)
	}
	tText, err := docxText(templatePath)
	if err != nil {
		return nil, fmt.Errorf("读取答案模板: %w", err)
	}
	pText, err := docxText(packPath)
	if err != nil {
		return nil, fmt.Errorf("读取参赛包: %w", err)
	}

	var questions []Question
	if err := json.Unmarshal([]byte(strings.TrimSpace(qText)), &questions); err != nil {
		return nil, fmt.Errorf("题目不是 JSON 数组: %w", err)
	}
	var tpl answerFile
	if err := json.Unmarshal([]byte(strings.TrimSpace(tText)), &tpl); err != nil {
		return nil, fmt.Errorf("答案模板不是 JSON: %w", err)
	}
	packID, issued := parsePack(pText)

	if packID == "" || tpl.PackageID == "" {
		return nil, fmt.Errorf("参赛包编号为空")
	}
	if packID != tpl.PackageID {
		return nil, fmt.Errorf("参赛包编号不一致: 参赛包=%s 模板=%s", packID, tpl.PackageID)
	}
	if packID != expectedPackageID {
		return nil, fmt.Errorf("参赛包编号是 %s，本包要求 %s", packID, expectedPackageID)
	}
	if len(questions) != len(tpl.Answers) {
		return nil, fmt.Errorf("题量不一致: 题目 %d 模板 %d", len(questions), len(tpl.Answers))
	}

	byID := make(map[string]Question, len(questions))
	for _, q := range questions {
		if q.ID == "" || strings.TrimSpace(q.Text) == "" {
			return nil, fmt.Errorf("题目缺少题号或自然语言输入")
		}
		if _, dup := byID[q.ID]; dup {
			return nil, fmt.Errorf("题目题号重复: %s", q.ID)
		}
		byID[q.ID] = q
	}
	ordered := make([]Question, 0, len(tpl.Answers))
	seen := make(map[string]bool, len(tpl.Answers))
	for _, a := range tpl.Answers {
		if seen[a.ID] {
			return nil, fmt.Errorf("模板题号重复: %s", a.ID)
		}
		seen[a.ID] = true
		q, ok := byID[a.ID]
		if !ok {
			return nil, fmt.Errorf("模板有题目中不存在的题号: %s", a.ID)
		}
		ordered = append(ordered, q)
	}
	if len(ordered) != len(byID) {
		return nil, fmt.Errorf("题目与模板题号集合不一致")
	}
	return &Bundle{PackageID: packID, Issued: issued, Questions: ordered}, nil
}

func parsePack(text string) (id, issued string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if id == "" {
			id = line
			continue
		}
		issued = line
		break
	}
	return id, issued
}

func docxText(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var doc *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return "", fmt.Errorf("%s 缺少 word/document.xml", path)
	}
	rc, err := doc.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return paragraphText(data)
}

func paragraphText(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var paras []string
	var b strings.Builder
	inP, inT := 0, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inP++
				if inP == 1 {
					b.Reset()
				}
			case "t":
				inT++
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				if inT > 0 {
					inT--
				}
			case "p":
				if inP > 0 {
					inP--
					if inP == 0 {
						paras = append(paras, b.String())
					}
				}
			}
		case xml.CharData:
			if inP > 0 && inT > 0 {
				b.Write(t)
			}
		}
	}
	return strings.Join(paras, "\n"), nil
}

func writeAnswer(path, name, packageID string, answers []Answer) error {
	payload := answerFile{Name: name, PackageID: packageID, Answers: answers}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
