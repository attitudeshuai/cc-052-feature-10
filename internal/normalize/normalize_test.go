package normalize

import "testing"

func TestName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"绿源蔬菜专业合作社", "绿源蔬菜专业合作社"},
		{"  绿源蔬菜专业合作社  ", "绿源蔬菜专业合作社"},
		{"绿源 蔬菜　专业合作社", "绿源蔬菜专业合作社"}, // 半角+全角空格
		{"ＡＢＣ农场", "ABC农场"},           // 全角字母
		{"abc农场", "ABC农场"},
		{"绿源（蔬菜）合作社", "绿源(蔬菜)合作社"}, // 全角括号
		{"\t绿源\n合作社 ", "绿源合作社"},
	}
	for _, c := range cases {
		if got := Name(c.in); got != c.want {
			t.Errorf("Name(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRegion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"寿光市", "寿光"},
		{"寿光县", "寿光"},
		{"寿光", "寿光"},
		{" 寿光市 ", "寿光"},
		{"寿光 市", "寿光"},
		{"shouguang", "SHOUGUANG"},
		{"Ｓｈｏｕｇｕａｎｇ", "SHOUGUANG"},
		{"370783", "370783"}, // 数字区划码保持不变
		{"昌乐县", "昌乐"},
		{"朝阳区", "朝阳"},
		{"某某自治县", "某某"},
		{"市", "市"}, // 防呆：不会归一成空串
	}
	for _, c := range cases {
		if got := Region(c.in); got != c.want {
			t.Errorf("Region(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCertNo(t *testing.T) {
	cases := []struct{ in, want string }{
		{" abc-123 ", "ABC-123"},
		{"ＡＢＣ１２３", "ABC123"},
		{"qs 3707 001", "QS3707001"},
		{"", ""},
	}
	for _, c := range cases {
		if got := CertNo(c.in); got != c.want {
			t.Errorf("CertNo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
