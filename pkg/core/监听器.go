package core

type EventFunc func(msg any)

// Listener 是支持反馈的全部类型的基础结构
type Listener struct {
	events []EventFunc
}

func (l *Listener) Listen(f EventFunc) {
	l.events = append(l.events, f)
}

func (l *Listener) Fire(msg any) {
	for _, f := range l.events {
		f(msg)
	}
}
