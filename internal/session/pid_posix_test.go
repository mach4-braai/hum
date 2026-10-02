//go:build !windows

package session

import (
	"os"
	"testing"
)

func TestProbeOwnerReportsWhatTheKernelAnswers(t *testing.T) {
	if got := realProbeOwner(os.Getpid()); got != ownerRunning {
		t.Errorf("realProbeOwner(own pid) = %v, want ownerRunning", got)
	}
	if got := realProbeOwner(1<<31 - 1); got != ownerExited {
		t.Errorf("realProbeOwner(unused pid) = %v, want ownerExited", got)
	}
}

func TestProbeOwnerCannotVouchForAnotherUsersProcess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may signal pid 1, so EPERM cannot be produced")
	}
	if got := realProbeOwner(1); got != ownerUnknown {
		t.Errorf("realProbeOwner(1) = %v, want ownerUnknown", got)
	}
}
