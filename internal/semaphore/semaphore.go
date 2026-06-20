package semaphore

type Semaphore chan struct{}

func New(permits int) Semaphore {
	if permits < 1 {
		panic("semaphore.New: permits must be >= 1")
	}
	s := make(chan struct{}, permits)
	for range permits {
		s <- struct{}{}
	}
	return s
}

func (s Semaphore) Acquire() {
	<-s
}

func (s Semaphore) Release() {
	s <- struct{}{}
}
