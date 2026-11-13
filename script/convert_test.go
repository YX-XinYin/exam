package main

import (
	"context"
	"strings"
	"testing"
)

type stubAssets struct{}

func (stubAssets) JenkinsFieldNames(context.Context, string) ([]string, error) {
	return diagnosticFields(jenkinsSample, "Jenkins", "Remoting"), nil
}

func (stubAssets) FaviconHash(context.Context, string) (string, error) {
	return "-100", nil
}

const jenkinsSample = "" +
	"Jenkins-Agent-Protocols: JNLP4-connect, Ping\n" +
	"Jenkins-Version: 2.289\n" +
	"Jenkins-Session: b4783dfa\n" +
	"Client: 172.17.0.1\n" +
	"Server: 172.17.0.3\n" +
	"Remoting-Minimum-Version: 3.14\n"

func TestBundleConvertsEveryQuestion(t *testing.T) {
	bundle, err := LoadBundle("../题目.docx", "../答案模板.docx", "../参赛包.docx")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.PackageID != expectedPackageID || len(bundle.Questions) != 100 {
		t.Fatalf("package=%s questions=%d", bundle.PackageID, len(bundle.Questions))
	}
	refused := map[string]bool{
		"M015-S001": true,
		"M091-S009": true,
		"M092-S007": true,
		"M093-S004": true,
		"M094-S004": true,
		"M095-S010": true,
	}
	for _, q := range bundle.Questions {
		got, err := Convert(context.Background(), q, stubAssets{})
		if err != nil {
			t.Fatalf("%s: %v", q.ID, err)
		}
		if strings.Contains(got, "\n") || strings.TrimSpace(got) == "" {
			t.Fatalf("%s: 查询不是单行非空内容: %q", q.ID, got)
		}
		if refused[q.ID] {
			if got != Unconvertible {
				t.Fatalf("%s: 应拒绝，得到 %s", q.ID, got)
			}
			continue
		}
		if got == Unconvertible || !strings.Contains(got, "=") {
			t.Fatalf("%s: 应生成查询，得到 %s", q.ID, got)
		}
		if strings.ContainsAny(got, "“”｜") {
			t.Fatalf("%s: 含有无法执行的符号: %s", q.ID, got)
		}
	}
}

func TestSamples(t *testing.T) {
	bundle, err := LoadBundle("../题目.docx", "../答案模板.docx", "../参赛包.docx")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Question{}
	for _, q := range bundle.Questions {
		byID[q.ID] = q
	}
	cases := []struct {
		id   string
		want string
	}{
		{"M001-S009", `ip="20.247.40.92"`},
		{"M002-S004", `ip="124.255.254.0/24"`},
		{"M079-S006", `ip="40.88.0.0/16"`},
		{"M080-S006", `(ip="47.96.0.1" || ip="47.96.0.2" || ip="47.96.0.3" || ip="47.96.0.4" || ip="47.96.0.5") && (port="80" || port="443")`},
		{"M088-S004", `country="HK" && server=="Apache" && asn!="4134"`},
		{"M089-S010", `app="HFS" && (port="80" || (port="8080" && status_code="200"))`},
		{"M091-S009", Unconvertible},
		{"M098-S001", `host="msp.gov.cn."`},
		{"M099-S001", `server="Apache/2.4.49"`},
		{"M100-S001", `cert.issuer.cn=="ZeroSSL ECC DV SSL CA 2" && cert.sn=="63bd83716214836ef80075356161eb06"`},
	}
	for _, tc := range cases {
		got, err := Convert(context.Background(), byID[tc.id], stubAssets{})
		if err != nil {
			t.Fatal(tc.id, err)
		}
		if got != tc.want {
			t.Fatalf("%s\n got %s\nwant %s", tc.id, got, tc.want)
		}
	}

	m078, err := Convert(context.Background(), byID["M078-S009"], stubAssets{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m078, `Symfony\\Component\\ErrorHandler\\Error\\FatalError`) {
		t.Fatalf("正文反斜杠未保留: %s", m078)
	}
	m090, err := Convert(context.Background(), byID["M090-S008"], stubAssets{})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`header="Content-Length: 1171"`, `header="Etag: \"6aa522a3-493\""`, `header="Server:"`, `header="Cache-Control:"`} {
		if !strings.Contains(m090, part) {
			t.Fatalf("响应头查询缺少 %s\n%s", part, m090)
		}
	}
	if strings.Contains(m090, "status_code") || strings.Contains(m090, "nginx") {
		t.Fatalf("不应限定状态码或 Server 值: %s", m090)
	}
	names := diagnosticFields(jenkinsSample, "Jenkins", "Remoting")
	if strings.Join(names, ",") != "Jenkins-Agent-Protocols,Jenkins-Version,Jenkins-Session,Remoting-Minimum-Version" {
		t.Fatalf("字段名: %v", names)
	}
}

func TestMurmurMatchesKnownVector(t *testing.T) {
	if got := murmur32([]byte("hello"), 0); got != 0x248bfa47 {
		t.Fatalf("murmur32(hello)=%x", got)
	}
	encoded := encodeBytes([]byte("hi"))
	if string(encoded) != "aGk=\n" {
		t.Fatalf("encodebytes=%q", encoded)
	}
}
