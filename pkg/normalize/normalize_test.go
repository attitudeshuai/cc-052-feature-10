package normalize

import "testing"

func TestNameKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"普通", "青山果蔬合作社", "青山果蔬"},
		{"带专业合作社", "青山果蔬专业合作社", "青山果蔬"},
		{"带农民专业合作社", "青山果蔬农民专业合作社", "青山果蔬"},
		{"全角字符与空格", "青 山果蔬农民专业合作社", "青山果蔬"},
		{"大小写与括号标点", "GreenHill 果蔬（合作社）", "greenhill果蔬"},
		{"家庭农场后缀", "绿源家庭农场", "绿源"},
		{"公司后缀", "绿源农业有限公司", "绿源农业"},
		{"无后缀保持", "青山一队", "青山一队"},
		{"首尾空白", "\t青山 果蔬合作社 \n", "青山果蔬"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NameKey(c.in); got != c.want {
				t.Errorf("NameKey(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNameKeyEquivalence(t *testing.T) {
	// 需求：只差一个字（后缀/空白/全半角）的写法必须归一到同一个 key
	group := []string{
		"青山果蔬农民专业合作社",
		"青山果蔬专业合作社",
		"青山果蔬合作社",
		"青山果蔬",
		" 青山果蔬合作社 ",
		"青山果蔬农民专业合作社",
	}
	base := NameKey(group[0])
	for _, g := range group[1:] {
		if NameKey(g) != base {
			t.Errorf("NameKey(%q)=%q != %q", g, NameKey(g), base)
		}
	}
	// 不同核心名不能并到一起
	if NameKey("青山果蔬合作社") == NameKey("青山果业合作社") {
		t.Error("青山果蔬 与 青山果业 不应判为同名")
	}
}

func TestCertNo(t *testing.T) {
	if CertNo(" nyc-001 ") != CertNo("ＮＹＣ－００１") {
		t.Error("资质号大小写/全半角变体应归一")
	}
	if CertNo("NYC001") != "NYC001" {
		t.Error("CertNo 应保留大写")
	}
}

func TestRegionCodeInput(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"130321", "130321"},
		{"１３０３２１", "130321"},
		{" 130321 ", "130321"},
		{"130321205001", "130321"}, // 村级 12 位截到县级
		{"130321205", "130321"},    // 乡级 9 位截到县级
		{"青龙县", "青龙县"},             // 非数字原样交给字典/别名解析
	}
	for _, c := range cases {
		if got := RegionCodeInput(c.in); got != c.want {
			t.Errorf("RegionCodeInput(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestCoreRegionName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"青龙满族自治县", "青龙"},
		{"鄂温克族自治旗", "鄂温克"},
		{"北京市", "北京"},
		{"张家口市", "张家口"},
		{"阿克苏地区", "阿克苏"},
		{"锡林郭勒盟", "锡林郭勒"},
		{"宽城满族自治县", "宽城"},
	}
	for _, c := range cases {
		if got := CoreRegionName(c.in); got != c.want {
			t.Errorf("CoreRegionName(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}
