// This deliberately racy executable verifies that the optimized CLI launcher
// still reports races and fails. It is never shipped or used as a Sidecar fake.
package main

func main() {
	var value int
	start := make(chan struct{})
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			<-start
			for range 1000 {
				value++
			}
			done <- struct{}{}
		}()
	}
	close(start)
	<-done
	<-done
	println(value)
}
