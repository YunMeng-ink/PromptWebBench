package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 是提示词版本号，格式为 v主.次（需求 §7.1）。
type Version struct {
	Major int
	Minor int
}

// ParseVersion 解析版本号，非法格式就地失败。
func ParseVersion(raw string) (Version, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "v") {
		return Version{}, fmt.Errorf("版本号 %q 缺少 v 前缀", raw)
	}
	majorText, minorText, ok := strings.Cut(strings.TrimPrefix(trimmed, "v"), ".")
	if !ok {
		return Version{}, fmt.Errorf("版本号 %q 缺少次版本号", raw)
	}
	major, err := strconv.Atoi(majorText)
	if err != nil || major < 1 {
		return Version{}, fmt.Errorf("版本号 %q 的主版本号非法", raw)
	}
	minor, err := strconv.Atoi(minorText)
	if err != nil || minor < 0 {
		return Version{}, fmt.Errorf("版本号 %q 的次版本号非法", raw)
	}
	return Version{Major: major, Minor: minor}, nil
}

// String 返回 v主.次 形式。
func (v Version) String() string {
	return "v" + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

// Next 返回下一个版本号（C-27）：普通修改次版本号 +1 且无上限（v1.9 之后为 v1.10）；
// 重大修改主版本号 +1 且次版本号归零（v1.3 之后为 v2.0）。
func (v Version) Next(major bool) Version {
	if major {
		return Version{Major: v.Major + 1, Minor: 0}
	}
	return Version{Major: v.Major, Minor: v.Minor + 1}
}

// FirstVersion 是新建提示词的初始版本。
func FirstVersion() Version { return Version{Major: 1, Minor: 0} }
