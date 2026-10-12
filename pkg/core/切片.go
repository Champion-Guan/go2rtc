package core

// 此代码复制自 Go 1.21，以兼容 Go 1.20。
// Windows 7 需要 Go 1.20 支持。

// Index 返回 v 在 s 中首次出现的索引，
// 不存在时返回 -1。
func Index[S ~[]E, E comparable](s S, v E) int {
	for i := range s {
		if v == s[i] {
			return i
		}
	}
	return -1
}

// Contains 判断 s 是否包含 v。
func Contains[S ~[]E, E comparable](s S, v E) bool {
	return Index(s, v) >= 0
}

type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

// Max 返回 x 中的最大值，x 为空时触发 panic。
// 对于浮点类型 E，Max 会传播 NaN，x 中任意 NaN
// 都会使输出为 NaN。
func Max[S ~[]E, E Ordered](x S) E {
	if len(x) < 1 {
		panic("slices.Max: empty list")
	}
	m := x[0]
	for i := 1; i < len(x); i++ {
		if x[i] > m {
			m = x[i]
		}
	}
	return m
}
