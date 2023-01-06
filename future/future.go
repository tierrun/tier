package future

type F[T any] struct {
	v   T
	err error
	c   chan struct{}
}

func Go[T any](f func() (T, error)) *F[T] {
	fu := &F[T]{c: make(chan struct{})}
	go fu.run(f)
	return fu
}

func (fu *F[T]) run(f func() (T, error)) {
	defer close(fu.c)
	fu.v, fu.err = f()
}

func (fu *F[T]) Get() (T, error) {
	<-fu.c
	return fu.v, fu.err
}
