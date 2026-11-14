package inference

const MaxConcurrent = 2

var slots = make(chan struct{}, MaxConcurrent)

// Acquire waits for one of the bounded CPU inference slots and returns its release function.
func Acquire() func() {
	slots <- struct{}{}
	return func() {
		<-slots
	}
}
