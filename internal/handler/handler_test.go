package handler

import "testing"

func TestRoleState(t *testing.T) {
	h := &Handler{
		ServiceRole: "shadow",
	}

	done := make(chan bool)

	go func() {
		for i := 0; i < 1000; i++ {
			h.SetRole("primary")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			h.SetRole("shadow")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			h.GetRole()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
