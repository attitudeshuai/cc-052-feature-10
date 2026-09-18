// Package normalize 负责合作社名称与行政区划输入的归一化。
// 建档与批量清洗共用同一套规则，保证「按归一后的地区与名称判重」。
package normalize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// 常见全角字符（含全角空格 U+3000）与半角的偏移
const fullWidthOffset = 0xFEE0

// foldRune 做全角转半角 + 小写折叠。
func foldRune(r rune) rune {
	if r == 0x3000 {
		r = ' '
	} else if r >= 0xFF01 && r <= 0xFF5E {
		r -= fullWidthOffset
	}
	return unicode.ToLower(r)
}

// Text 做基础字符归一：全角转半角 + 小写折叠 + 去除所有空白。
// 名称内部空格也去掉，避免「青山 果蔬」与「青山果蔬」判成两家。
func Text(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		r = foldRune(r)
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// CertNo 归一资质编号：去空白、全角转半角、转大写。
func CertNo(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0x3000 {
			r = ' '
		} else if r >= 0xFF01 && r <= 0xFF5E {
			r -= fullWidthOffset
		}
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// 名称末尾常见组织形式后缀（长的在前），归一名称时剥除。
// 只剥真正的组织形式词，避免把行业词剥掉造成误并。
var suffixes = []string{
	"农民专业合作社",
	"专业合作社",
	"合作社",
	"家庭农场",
	"有限责任公司",
	"有限公司",
	"公司",
	"协会",
}

// NameKey 返回合作社名称的判重键：
// 基础归一 → 去标点 → 反复剥组织形式后缀（如「XX合作社有限公司」）。
func NameKey(name string) string {
	key := stripPunct(Text(name))
	for {
		before := key
		for _, suf := range suffixes {
			if len(key) > len(suf) && strings.HasSuffix(key, suf) {
				key = key[:len(key)-len(suf)]
			}
		}
		if key == before {
			break
		}
	}
	return key
}

// stripPunct 删除所有标点/符号类 rune（含中文标点）。
func stripPunct(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsDigits 判断字符串是否全部由数字组成且非空。
func IsDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// RegionCodeInput 对用户填写的地区编码做基础归一：
// 全角数字转半角、去空白；9/12 位乡级/村级码截断到 6 位县级码（判重粒度到县）。
// 进一步的字典解析在 service 层完成。
func RegionCodeInput(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r == 0x3000 {
			r = ' '
		} else if r >= 0xFF10 && r <= 0xFF19 {
			r -= fullWidthOffset
		}
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	if utf8.RuneCountInString(s) >= 9 && IsDigits(s) {
		return s[:6]
	}
	return s
}

// 行政区划后缀：民族自治类 + 普通类；每轮取最长匹配，避免
// 「满族自治县」被更短的「族自治县」先截成「青龙满」。
var regionSuffixes = []string{
	"各族自治县", "各民族自治县",
	"莫力达瓦达斡尔族自治旗",
	"回族自治县", "满族自治县", "蒙古族自治县", "藏族自治县", "彝族自治县",
	"回族自治旗", "满族自治旗", "蒙古族自治旗",
	"鄂温克族自治旗", "鄂伦春族自治旗", "达斡尔族自治旗",
	"族自治县", "族自治旗",
	"自治县", "自治旗", "自治区", "自治州",
	"地区",
	"市辖区",
	"区", "县", "旗", "市", "盟",
}

// CoreRegionName 从行政区划标准名提取核心名，用于别名表未命中时兜底匹配：
// 「青龙满族自治县」→「青龙」、「鄂温克族自治旗」→「鄂温克」、「北京市」→「北京」。
func CoreRegionName(name string) string {
	s := stripPunct(Text(name))
	for {
		match := ""
		for _, w := range regionSuffixes {
			if len(s) > len(w) && strings.HasSuffix(s, w) && len(w) > len(match) {
				match = w
			}
		}
		if match == "" {
			return s
		}
		s = s[:len(s)-len(match)]
	}
}
