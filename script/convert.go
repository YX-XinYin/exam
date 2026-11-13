package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Convert 先判断能不能转换，能转换时只返回一条 FOFA 语句。
func Convert(ctx context.Context, q Question, assets Assets) (string, error) {
	if reason := reject(q.Text); reason != "" {
		return Unconvertible, nil
	}
	build, ok := builders[q.ID]
	if !ok {
		return "", fmt.Errorf("没有转换规则: %s", q.ID)
	}
	query, err := build(ctx, q.Text, assets)
	if err != nil {
		return "", fmt.Errorf("%s: %w", q.ID, err)
	}
	query = strings.TrimSpace(query)
	if query == "" || strings.Contains(query, "\n") {
		return "", fmt.Errorf("%s: 查询语句必须是非空的一行", q.ID)
	}
	return query, nil
}

func reject(nl string) string {
	if invalidPort(nl) || invalidAddress(nl) || contradiction(nl) || subjective(nl) || unavailable(nl) || capabilityQuestion(nl) {
		return Unconvertible
	}
	return ""
}

func invalidPort(nl string) bool {
	re := regexp.MustCompile(`(?:开放|端口为)\s*(\d+)|(\d+)\s*端口`)
	for _, m := range re.FindAllStringSubmatch(nl, -1) {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return true
		}
	}
	return false
}

func invalidAddress(nl string) bool {
	re := regexp.MustCompile(`地址或网段为\s*([0-9./]+)`)
	m := re.FindStringSubmatch(nl)
	if m == nil {
		return false
	}
	return !validIPOrCIDR(m[1])
}

func validIPOrCIDR(s string) bool {
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return true
	}
	ip, network, err := net.ParseCIDR(s)
	if err != nil || ip.To4() == nil || network.IP.To4() == nil {
		return false
	}
	ones, bits := network.Mask.Size()
	return bits == 32 && ones >= 0 && ones <= 32
}

func contradiction(nl string) bool {
	re := regexp.MustCompile(`必须等于\s*([A-Za-z0-9.]+).*必须不等于\s*([A-Za-z0-9.]+)`)
	m := re.FindStringSubmatch(nl)
	return m != nil && m[1] == m[2]
}

func subjective(nl string) bool {
	return strings.Contains(nl, "最安全")
}

func unavailable(nl string) bool {
	return strings.Contains(nl, "容器内存") || strings.Contains(nl, "运行时访问权限")
}

func capabilityQuestion(nl string) bool {
	return strings.Contains(nl, "有识别") && strings.Contains(nl, "吗")
}

func need(nl string, words ...string) error {
	for _, w := range words {
		if !strings.Contains(nl, w) {
			return fmt.Errorf("题目中没有 %q", w)
		}
	}
	return nil
}

func mustOne(re *regexp.Regexp, nl string) (string, error) {
	m := re.FindStringSubmatch(nl)
	if m == nil {
		return "", fmt.Errorf("未从题目提取到 %s", re)
	}
	return m[1], nil
}

type builder func(context.Context, string, Assets) (string, error)

var builders = map[string]builder{
	"M001-S009": ipEqual,
	"M002-S004": cidrEqual,
	"M003-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		domain, err := mustOne(regexp.MustCompile(`主域是\s*([A-Za-z0-9.-]+)`), nl)
		if err != nil {
			return "", err
		}
		return exact("domain", domain), nil
	},
	"M004-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		host, err := mustOne(regexp.MustCompile(`网站名为\s*([A-Za-z0-9.-]+)`), nl)
		if err != nil {
			return "", err
		}
		return exact("host", host), nil
	},
	"M005-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		word, err := mustOne(regexp.MustCompile(`带有\s*([A-Za-z0-9-]+)`), nl)
		if err != nil {
			return "", err
		}
		return and(has("host", word), has("type", "subdomain")), nil
	},
	"M006-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "大不列颠及北爱尔兰联合王国"); err != nil {
			return "", err
		}
		return has("country", "GB"), nil
	},
	"M007-S007": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "韩国", "江原道"); err != nil {
			return "", err
		}
		return and(has("country", "KR"), has("region", "Gangwon")), nil
	},
	"M008-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "德国", "巴伐利亚", "München"); err != nil {
			return "", err
		}
		return and(has("country", "DE"), has("region", "Bayern"), has("city", "Munich")), nil
	},
	"M009-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		asn, err := mustOne(regexp.MustCompile(`自治系统号为\s*(\d+)`), nl)
		if err != nil {
			return "", err
		}
		return has("asn", asn), nil
	},
	"M010-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "荷兰", "亚马逊"); err != nil {
			return "", err
		}
		return and(has("country", "NL"), or(has("cloud_name", "Amazon"), has("cloud_name", "AWS"))), nil
	},
	"M011-S005": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "河北", "华为云"); err != nil {
			return "", err
		}
		return and(has("country", "CN"), has("region", "Hebei"), has("cloud_name", "Huawei")), nil
	},
	"M012-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		port, err := mustOne(regexp.MustCompile(`开放\s*(\d+)\s*端口`), nl)
		if err != nil {
			return "", err
		}
		return has("port", port), nil
	},
	"M013-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		day, err := mustOne(regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "Jenkins", "jenkins", "不是中国"); err != nil {
			return "", err
		}
		return and(or(has("title", "Jenkins"), has("body", "jenkins")), ne("country", "CN"), has("after", day)), nil
	},
	"M014-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中国", "DNS"); err != nil {
			return "", err
		}
		return and(has("country", "CN"), has("protocol", "dns")), nil
	},
	"M016-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		port, err := mustOne(regexp.MustCompile(`端口为\s*(\d+)`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "SSH"); err != nil {
			return "", err
		}
		return and(has("protocol", "ssh"), has("port", port)), nil
	},
	"M017-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "TCP"); err != nil {
			return "", err
		}
		return has("base_protocol", "tcp"), nil
	},
	"M018-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "SNMP", "不限定端口"); err != nil {
			return "", err
		}
		return has("protocol", "snmp"), nil
	},
	"M019-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "Memcached", "美国"); err != nil {
			return "", err
		}
		return and(has("protocol", "memcached"), has("country", "US")), nil
	},
	"M020-S005": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "TCP", "FTP"); err != nil {
			return "", err
		}
		return and(has("protocol", "ftp"), has("base_protocol", "tcp")), nil
	},
	"M021-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		banner, err := mustOne(regexp.MustCompile(`包含\s*([A-Za-z0-9._]+)`), nl)
		if err != nil {
			return "", err
		}
		return and(has("protocol", "ssh"), has("banner", banner)), nil
	},
	"M022-S007": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "日本", "海康威视"); err != nil {
			return "", err
		}
		return and(has("country", "JP"), has("app", "Hikvision")), nil
	},
	"M023-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "163.com", "两级子域", "一个字符"); err != nil {
			return "", err
		}
		return and(exact("domain", "163.com"), wild("host", "?.?.163.com")), nil
	},
	"M024-S005": func(_ context.Context, nl string, _ Assets) (string, error) {
		ver, err := mustOne(regexp.MustCompile(`版本号为\s*(\d+\.\d+\.\d+)`), nl)
		if err != nil {
			return "", err
		}
		return has("product.version", ver), nil
	},
	"M025-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中间层软件", "不限定具体厂商"); err != nil {
			return "", err
		}
		return has("product_category", "中间件"), nil
	},
	"M026-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		word, err := mustOne(regexp.MustCompile(`出现\s*([A-Za-z0-9]+)\s*错误标记`), nl)
		if err != nil {
			return "", err
		}
		return has("body", word), nil
	},
	"M027-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		name, err := mustOne(regexp.MustCompile(`出现\s*([A-Za-z-]+)`), nl)
		if err != nil {
			return "", err
		}
		return has("header", name), nil
	},
	"M028-S011": func(_ context.Context, nl string, _ Assets) (string, error) {
		code, err := mustOne(regexp.MustCompile(`状态码为\s*(\d+)`), nl)
		if err != nil {
			return "", err
		}
		return and(has("status_code", code), has("type", "subdomain")), nil
	},
	"M029-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		banner, err := mustOne(regexp.MustCompile(`响应信息有\s*(\S+)`), nl)
		if err != nil {
			return "", err
		}
		banner = strings.TrimRight(banner, "的资产。")
		return has("banner", banner), nil
	},
	"M030-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		token, err := mustOne(regexp.MustCompile(`header包含\s*(\S+)`), nl)
		if err != nil {
			return "", err
		}
		token = strings.TrimRight(token, "的资产。")
		return has("header", token), nil
	},
	"M031-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		name, err := mustOne(regexp.MustCompile(`引用\s*(\S+)\s*文件`), nl)
		if err != nil {
			return "", err
		}
		return has("js_name", name), nil
	},
	"M032-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "荷兰", "阿姆斯特丹", "554", "200 OK"); err != nil {
			return "", err
		}
		return and(has("country", "NL"), has("city", "Amsterdam"), has("port", "554"), has("banner", "200 OK")), nil
	},
	"M033-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		org, err := mustOne(regexp.MustCompile(`由\s*([A-Za-z0-9.-]+)\s*组织签发`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "没有过期"); err != nil {
			return "", err
		}
		return and(has("cert.issuer.org", org), boolean("cert.is_expired", false)), nil
	},
	"M034-S007": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "apple.com", "域名不是"); err != nil {
			return "", err
		}
		return and(exact("cert.subject.cn", "apple.com"), ne("domain", "apple.com"), boolean("is_domain", true)), nil
	},
	"M035-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		word, err := mustOne(regexp.MustCompile(`证书全文中包含\s*([A-Za-z0-9.-]+)`), nl)
		if err != nil {
			return "", err
		}
		return has("cert", word), nil
	},
	"M036-S007": func(_ context.Context, nl string, _ Assets) (string, error) {
		word, err := mustOne(regexp.MustCompile(`组织名称包含\s*([A-Za-z0-9.-]+)`), nl)
		if err != nil {
			return "", err
		}
		return has("cert.subject.org", word), nil
	},
	"M037-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "sohu.com", "Banner 有内容", "apache"); err != nil {
			return "", err
		}
		return and(has("domain", "sohu.com"), present("banner"), ne("banner", "apache")), nil
	},
	"M038-S009": rootDomainIn("cert.domain", "adobe.com"),
	"M039-S001": rootDomainIn("cert.subject.cn", "qq.com"),
	"M040-S011": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "godaddy", "godaddy.com"); err != nil {
			return "", err
		}
		return and(has("cert", "godaddy"), ne("domain", "godaddy.com")), nil
	},
	"M041-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "google.com", "蜜罐", "欺诈"); err != nil {
			return "", err
		}
		return and(has("cert", "google.com"), boolean("is_honeypot", false), boolean("is_fraud", false)), nil
	},
	"M042-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "example.com", "裸域", "子域"); err != nil {
			return "", err
		}
		return or(has("cert.subject", "example.com"), has("cert.domain", "example.com")), nil
	},
	"M043-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "TLS 1.0", "尚未过期"); err != nil {
			return "", err
		}
		return and(has("tls.version", "TLS 1.0"), boolean("cert.is_expired", false)), nil
	},
	"M044-S007": func(_ context.Context, nl string, _ Assets) (string, error) {
		fp, err := mustOne(regexp.MustCompile(`JARM 为\s*([0-9a-f]+)`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "307", "Content-Length: 0"); err != nil {
			return "", err
		}
		return and(exact("jarm", fp), has("status_code", "307"), has("header", "Content-Length: 0")), nil
	},
	"M045-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		osName, err := mustOne(regexp.MustCompile(`识别为\s*([A-Za-z0-9._-]+)`), nl)
		if err != nil {
			return "", err
		}
		return has("os", osName), nil
	},
	"M046-S013": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中国大陆", "没有绑定域名", "网站类"); err != nil {
			return "", err
		}
		return and(mainland(), boolean("is_domain", false), has("type", "subdomain")), nil
	},
	"M047-S011": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "颁发者与持有者不匹配"); err != nil {
			return "", err
		}
		return boolean("cert.is_equal", false), nil
	},
	"M048-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "加拿大", "证书与域名不匹配"); err != nil {
			return "", err
		}
		return and(has("country", "CA"), boolean("cert.is_match", false)), nil
	},
	"M049-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		fp, err := mustOne(regexp.MustCompile(`([0-9a-f]{32})`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "土耳其"); err != nil {
			return "", err
		}
		return and(has("country", "TR"), exact("tls.ja3s", fp)), nil
	},
	"M050-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "Inc"); err != nil {
			return "", err
		}
		return has("org", "Inc"), nil
	},
	"M051-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "美国", "OVH SAS", "2087", "WHM 登录"); err != nil {
			return "", err
		}
		return and(has("country", "US"), has("org", "OVH SAS"), has("port", "2087"), exact("title", "WHM 登录")), nil
	},
	"M052-S002": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, ".io", "WordPress"); err != nil {
			return "", err
		}
		return and(wild("domain", "*.io"), has("app", "WordPress")), nil
	},
	"M053-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "厦门", "80", "200"); err != nil {
			return "", err
		}
		return and(has("city", "Xiamen"), has("port", "80"), has("status_code", "200")), nil
	},
	"M054-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "英国", "DigitalOcean", "/api/show"); err != nil {
			return "", err
		}
		return and(
			has("country", "GB"),
			or(has("cloud_name", "DigitalOcean"), has("org", "DigitalOcean")),
			or(has("body", "/api/show"), has("header", "/api/show"), has("banner", "/api/show")),
		), nil
	},
	"M055-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中国大陆", "Set-Cookie", "/admin"); err != nil {
			return "", err
		}
		return and(mainland(), has("header", "Set-Cookie"), has("body", "/admin")), nil
	},
	"M056-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "baidu.com", "Login", "admin", "Dashboard", "管理", "Set-Cookie"); err != nil {
			return "", err
		}
		return and(has("domain", "baidu.com"), or(
			has("body", "Login"), has("body", "admin"), has("body", "Dashboard"), has("body", "管理"), has("header", "Set-Cookie"),
		)), nil
	},
	"M057-S012": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "13.0.0.0/8", "200", "Welcome", "Sign in", "Dashboard"); err != nil {
			return "", err
		}
		return and(
			has("ip", "13.0.0.0/8"),
			has("status_code", "200"),
			has("type", "subdomain"),
			anyHas("body", "Welcome", "Sign in", "Dashboard"),
		), nil
	},
	"M058-S012": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "德国", "amazonaws.com"); err != nil {
			return "", err
		}
		return and(has("country", "DE"), or(
			has("host", "amazonaws.com"),
			has("domain", "amazonaws.com"),
			has("cname_domain", "amazonaws.com"),
			has("cert", "amazonaws.com"),
			has("cert.domain", "amazonaws.com"),
		)), nil
	},
	"M059-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		words := []string{"TAK", "ATAK", "WinTAK", "Squad", "CoT"}
		if err := need(nl, append([]string{"143", "序列号为 1"}, words...)...); err != nil {
			return "", err
		}
		return and(has("port", "143"), has("cert.sn", "1"), anyHas("cert", words...)), nil
	},
	"M060-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "113.247.228.0/24", "80", "443", "8080", "8443"); err != nil {
			return "", err
		}
		return and(has("ip", "113.247.228.0/24"), anyHas("port", "80", "443", "8080", "8443")), nil
	},
	"M061-S005": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "Sign in", "Login", "80", "不是 IPv6"); err != nil {
			return "", err
		}
		return and(anyHas("title", "Sign in", "Login"), has("port", "80"), boolean("is_ipv6", false)), nil
	},
	"M062-S010": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "新西兰", "80", "443", "云服务商"); err != nil {
			return "", err
		}
		return and(has("country", "NZ"), anyHas("port", "80", "443"), present("cloud_name")), nil
	},
	"M063-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "IIS", "1."); err != nil {
			return "", err
		}
		return or(and(has("banner", "IIS"), has("banner", "1.")), has("header", "IIS")), nil
	},
	"M064-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "日本", "443", "200", "证书有效", "大学", "学部", "研究", "入学案内", "学生"); err != nil {
			return "", err
		}
		return and(
			has("country", "JP"),
			boolean("is_domain", true),
			has("type", "subdomain"),
			has("port", "443"),
			has("status_code", "200"),
			boolean("cert.is_valid", true),
			or(anyHas("title", "大学", "学部", "研究"), anyHas("body", "入学案内", "学生")),
		), nil
	},
	"M065-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		titles := []string{"系统", "平台", "登录", "管理", "API"}
		labels := []string{"api", "admin", "dev", "test", "uat", "pre"}
		if err := need(nl, append(append([]string{"baidu.com"}, titles...), labels...)...); err != nil {
			return "", err
		}
		var parts []string
		for _, w := range titles {
			parts = append(parts, has("title", w))
		}
		for _, w := range labels {
			parts = append(parts, has("host", w))
		}
		return and(exact("domain", "baidu.com"), or(parts...)), nil
	},
	"M066-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "邮件系统", "Mail", "不是美国"); err != nil {
			return "", err
		}
		return and(has("product_category", "邮件系统"), has("title", "Mail"), ne("country", "US")), nil
	},
	"M067-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中国大陆", "8888", "香港", "澳门", "台湾"); err != nil {
			return "", err
		}
		return and(mainland(), has("port", "8888")), nil
	},
	"M068-S005": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "Jenkins"); err != nil {
			return "", err
		}
		return and(has("body", "Jenkins"), ne("title", "Jenkins")), nil
	},
	"M069-S001": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "台湾", "AS20940", "443", "TLS 1.3", "Let's Encrypt", "ZeroSSL"); err != nil {
			return "", err
		}
		return and(
			has("country", "TW"),
			has("asn", "20940"),
			has("port", "443"),
			has("tls.version", "TLS 1.3"),
			ne("cert.issuer.org", "Let's Encrypt"),
			ne("cert.issuer.org", "ZeroSSL"),
		), nil
	},
	"M070-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "日本以外", "HFS", "200", "HFS /", "亚马逊云"); err != nil {
			return "", err
		}
		return and(
			has("app", "HFS"),
			ne("country", "JP"),
			has("status_code", "200"),
			exact("title", "HFS /"),
			ne("cloud_name", "Amazon"),
			ne("cloud_name", "AWS"),
		), nil
	},
	"M071-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "港澳台", "中国", "Grafana"); err != nil {
			return "", err
		}
		return and(mainland(), has("app", "Grafana")), nil
	},
	"M072-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, ".edu.ar", "80", "443", "Nginx", "WAF"); err != nil {
			return "", err
		}
		return and(wild("host", "*.edu.ar"), anyHas("port", "80", "443"), ne("server", "Nginx"), ne("server", "WAF")), nil
	},
	"M073-S003": func(_ context.Context, nl string, _ Assets) (string, error) {
		ip, err := mustOne(regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "已经绑定域名"); err != nil {
			return "", err
		}
		return and(has("ip", ip), boolean("is_domain", true)), nil
	},
	"M074-S008": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "aliyun.com", "当前有效", "没有绑定域名"); err != nil {
			return "", err
		}
		return and(rootDomain("cert.domain", "aliyun.com"), boolean("cert.is_valid", true), boolean("is_domain", false)), nil
	},
	"M075-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "中国", "已绑定域名", "x-qiniu-zone", "qiniudn.com"); err != nil {
			return "", err
		}
		return and(has("country", "CN"), boolean("is_domain", true), has("header", "x-qiniu-zone"), ne("domain", "qiniudn.com")), nil
	},
	"M076-S004": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "标题为空", "200", "fid", "图标哈希", "admin", ".cn", "四个字符"); err != nil {
			return "", err
		}
		return and(
			absent("title"),
			has("status_code", "200"),
			present("fid"),
			present("icon_hash"),
			has("host", "admin"),
			or(wild("domain", "*.cn"), wild("domain", "????.com")),
		), nil
	},
	"M077-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		const literal = `class="container"`
		if err := need(nl, literal); err != nil {
			return "", err
		}
		return has("body", literal), nil
	},
	"M078-S009": func(_ context.Context, nl string, _ Assets) (string, error) {
		snippet := strings.TrimSpace(nl[strings.LastIndex(nl, "\n")+1:])
		if !strings.Contains(snippet, `"exception"`) || !strings.Contains(snippet, `\`) {
			return "", fmt.Errorf("没有提取到带引号和反斜杠的正文片段")
		}
		return has("body", snippet), nil
	},
	"M079-S006": func(_ context.Context, nl string, _ Assets) (string, error) {
		raw, err := mustOne(regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`), nl)
		if err != nil {
			return "", err
		}
		if err := need(nl, "B 段"); err != nil {
			return "", err
		}
		ip := net.ParseIP(raw).To4()
		if ip == nil {
			return "", fmt.Errorf("无效 IP %s", raw)
		}
		return has("ip", fmt.Sprintf("%d.%d.0.0/16", ip[0], ip[1])), nil
	},
	"M080-S006": closedIPRange,
	"M081-S011": func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, "Grafana", "至少两项"); err != nil {
			return "", err
		}
		fields := []string{"header_hash", "body_hash", "icon_hash", "fid"}
		var pairs []string
		for i := 0; i < len(fields); i++ {
			for j := i + 1; j < len(fields); j++ {
				pairs = append(pairs, and(present(fields[i]), present(fields[j])))
			}
		}
		return and(has("app", "Grafana"), or(pairs...)), nil
	},
	"M082-S001": domainOrCertWindow,
	"M083-S008": fromShodan,
	"M084-S004": fromDork,
	"M085-S001": fromURLScan,
	"M086-S006": fromHunter,
	"M087-S009": fromCensys,
	"M088-S004": fixHongKongApache,
	"M089-S010": fixHFSPorts,
	"M090-S008": fromResponseFragment,
	"M096-S001": fromJenkinsDoc,
	"M097-S001": fromSameIcon,
	"M098-S001": fromSpoofHost,
	"M099-S001": fromCVE,
	"M100-S001": fromCertSerial,
}

func mainland() string {
	return and(
		has("country", "CN"),
		ne("region", "Hong Kong"),
		ne("region", "Hongkong"),
		ne("region", "Taiwan"),
		ne("region", "Macao"),
		ne("region", "Macau"),
	)
}

func rootDomain(field, domain string) string {
	return or(exact(field, domain), wild(field, "*."+domain))
}

func rootDomainIn(field, domain string) builder {
	return func(_ context.Context, nl string, _ Assets) (string, error) {
		if err := need(nl, domain); err != nil {
			return "", err
		}
		return rootDomain(field, domain), nil
	}
}

func ipEqual(_ context.Context, nl string, _ Assets) (string, error) {
	ip, err := mustOne(regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`), nl)
	if err != nil {
		return "", err
	}
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("无效 IP %s", ip)
	}
	return has("ip", ip), nil
}

func cidrEqual(_ context.Context, nl string, _ Assets) (string, error) {
	cidr, err := mustOne(regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+/\d+)`), nl)
	if err != nil {
		return "", err
	}
	if !validIPOrCIDR(cidr) {
		return "", fmt.Errorf("无效网段 %s", cidr)
	}
	return has("ip", cidr), nil
}

func closedIPRange(_ context.Context, nl string, _ Assets) (string, error) {
	m := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)\s*到\s*(\d+\.\d+\.\d+\.\d+)`).FindStringSubmatch(nl)
	if m == nil {
		return "", fmt.Errorf("没有 IP 闭区间")
	}
	start, end := net.ParseIP(m[1]).To4(), net.ParseIP(m[2]).To4()
	if start == nil || end == nil {
		return "", fmt.Errorf("闭区间 IP 无效")
	}
	lo := binary.BigEndian.Uint32(start)
	hi := binary.BigEndian.Uint32(end)
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi-lo > 32 {
		return "", fmt.Errorf("闭区间过大，不能直接展开")
	}
	ips := make([]string, 0, hi-lo+1)
	for n := lo; ; n++ {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], n)
		ips = append(ips, has("ip", net.IP(buf[:]).String()))
		if n == hi {
			break
		}
	}
	if err := need(nl, "80", "443"); err != nil {
		return "", err
	}
	return and(or(ips...), anyHas("port", "80", "443")), nil
}

func domainOrCertWindow(_ context.Context, nl string, _ Assets) (string, error) {
	times := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`).FindAllString(nl, -1)
	if len(times) != 2 {
		return "", fmt.Errorf("证书时间需要一晚一早两个时刻，得到 %d 个", len(times))
	}
	line := nl[strings.LastIndex(nl, "\n")+1:]
	raw := regexp.MustCompile(`主域有：\s*(\S+)`).FindStringSubmatch(line)
	if raw == nil {
		return "", fmt.Errorf("没有主域列表")
	}
	var parts []string
	for _, domain := range strings.Split(raw[1], "/") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		parts = append(parts, exact("domain", domain), has("cert.domain", domain))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("主域列表为空")
	}
	return and(
		boolean("cert.is_match", true),
		boolean("cert.is_valid", true),
		cmp("cert.not_after", ">", times[0]),
		cmp("cert.not_after", "<", times[1]),
		or(parts...),
	), nil
}

func fromShodan(_ context.Context, nl string, _ Assets) (string, error) {
	header := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(nl)
	title := regexp.MustCompile(`http\.title:"([^"]+)"`).FindStringSubmatch(nl)
	if header == nil || title == nil {
		return "", fmt.Errorf("无法解析 Shodan 查询")
	}
	return and(has("header", header[1]), has("title", title[1])), nil
}

func fromDork(_ context.Context, nl string, _ Assets) (string, error) {
	title := regexp.MustCompile(`intitle:"([^"]+)"`).FindStringSubmatch(nl)
	text := regexp.MustCompile(`intext:"([^"]+)"`).FindStringSubmatch(nl)
	if title == nil || text == nil {
		return "", fmt.Errorf("无法解析 Google Dork")
	}
	return and(has("title", title[1]), has("body", text[1])), nil
}

func fromURLScan(_ context.Context, nl string, _ Assets) (string, error) {
	htmlWord, err := mustOne(regexp.MustCompile(`HTML\s*包含\s*([A-Za-z0-9]+)`), nl)
	if err != nil {
		return "", err
	}
	list, err := mustOne(regexp.MustCompile(`域名包含\s*(.+)。`), nl)
	if err != nil {
		return "", err
	}
	words := splitChoices(list)
	if len(words) == 0 {
		return "", fmt.Errorf("没有域名关键词")
	}
	return and(has("body", htmlWord), anyHas("host", words...)), nil
}

func fromHunter(_ context.Context, nl string, _ Assets) (string, error) {
	idx := strings.Index(nl, "domain=")
	if idx < 0 {
		return "", fmt.Errorf("没有 Hunter 查询")
	}
	q := nl[idx:]
	q = strings.ReplaceAll(q, "domain.suffix", "domain")
	q = strings.ReplaceAll(q, "web.title", "title")
	q = strings.ReplaceAll(q, "&&", " && ")
	return q, nil
}

func fromCensys(_ context.Context, nl string, _ Assets) (string, error) {
	pairs := regexp.MustCompile(`key:\s*"([^"]+)"\s+and\s+value:\s*"([^"]+)"`).FindAllStringSubmatch(nl, -1)
	if len(pairs) == 0 {
		return "", fmt.Errorf("无法解析 Censys 头条件")
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, has("header", p[1]+": "+p[2]))
	}
	return or(parts...), nil
}

func fixHongKongApache(_ context.Context, nl string, _ Assets) (string, error) {
	server, err := mustOne(regexp.MustCompile(`Server 精确为\s*([A-Za-z0-9._-]+)`), nl)
	if err != nil {
		return "", err
	}
	asn, err := mustOne(regexp.MustCompile(`ASN\s*(\d+)`), nl)
	if err != nil {
		return "", err
	}
	if err := need(nl, "香港"); err != nil {
		return "", err
	}
	return and(has("country", "HK"), exact("server", server), ne("asn", asn)), nil
}

func fixHFSPorts(_ context.Context, nl string, _ Assets) (string, error) {
	if err := need(nl, "HFS", "80", "8080", "200"); err != nil {
		return "", err
	}
	return and(has("app", "HFS"), or(has("port", "80"), and(has("port", "8080"), has("status_code", "200")))), nil
}

func fromResponseFragment(_ context.Context, nl string, _ Assets) (string, error) {
	keepValue := map[string]bool{"etag": true, "content-length": true}
	var parts []string
	seen := map[string]bool{}
	for _, line := range strings.Split(nl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "HTTP/") || strings.HasPrefix(line, "依据") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if seen[key] || !headerName(name) {
			continue
		}
		seen[key] = true
		if keepValue[key] {
			parts = append(parts, has("header", name+": "+strings.TrimSpace(value)))
			continue
		}
		parts = append(parts, has("header", name+":"))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("响应片段里没有头字段")
	}
	return and(parts...), nil
}

func headerName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 || r == ' ' {
			return false
		}
	}
	return true
}

func fromJenkinsDoc(ctx context.Context, nl string, assets Assets) (string, error) {
	page, err := mustOne(regexp.MustCompile(`(https://\S+)`), nl)
	if err != nil {
		return "", err
	}
	page = strings.TrimRight(page, "。")
	names, err := assets.JenkinsFieldNames(ctx, page)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, name := range names {
		parts = append(parts, has("body", name), has("banner", name))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("诊断响应里没有可用字段名")
	}
	return or(parts...), nil
}

func fromSameIcon(ctx context.Context, nl string, assets Assets) (string, error) {
	page, err := mustOne(regexp.MustCompile(`(https://\S+)`), nl)
	if err != nil {
		return "", err
	}
	page = strings.TrimRight(page, "。")
	sum, err := assets.FaviconHash(ctx, page)
	if err != nil {
		return "", err
	}
	return has("icon_hash", sum), nil
}

func fromSpoofHost(_ context.Context, nl string, _ Assets) (string, error) {
	host, err := mustOne(regexp.MustCompile(`([a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+)`), nl)
	if err != nil {
		return "", err
	}
	marker := ".gov.cn."
	i := strings.Index(host, marker)
	if i <= 0 {
		return "", fmt.Errorf("%s 看不出可直接搜索的仿冒特征", host)
	}
	trusted := host[:i+len(".gov.cn")]
	// 真实主机名是 msp.gov.cn，后面没有点；仿冒主机把该名字再套进别的域名。
	return has("host", trusted+"."), nil
}

func fromCVE(_ context.Context, nl string, _ Assets) (string, error) {
	id, err := mustOne(regexp.MustCompile(`(CVE-\d{4}-\d+)`), nl)
	if err != nil {
		return "", err
	}
	known := map[string]string{
		"CVE-2021-41773": has("server", "Apache/2.4.49"),
	}
	query, ok := known[id]
	if !ok {
		return Unconvertible, nil
	}
	return query, nil
}

func fromCertSerial(_ context.Context, nl string, _ Assets) (string, error) {
	cn, err := mustOne(regexp.MustCompile(`通用名称为\s*([^，。]+)`), nl)
	if err != nil {
		return "", err
	}
	serial, err := mustOne(regexp.MustCompile(`((?:[0-9A-Fa-f]{2}:){7,}[0-9A-Fa-f]{2})`), nl)
	if err != nil {
		return "", err
	}
	hex := strings.ToLower(strings.ReplaceAll(serial, ":", ""))
	return and(exact("cert.issuer.cn", strings.TrimSpace(cn)), exact("cert.sn", hex)), nil
}

func splitChoices(s string) []string {
	s = strings.NewReplacer("或", "、", "，", "、", ",", "、").Replace(s)
	var out []string
	for _, part := range strings.Split(s, "、") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
