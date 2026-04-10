package lane

import (
	"errors"
	"sync"
	"testing"
)

func TestDetectsSameBranchSameModuleCollisions(t *testing.T) {
	wta := "wt-a"
	wtb := "wt-b"
	collisions := DetectBranchLockCollisions([]BranchLockIntent{
		{LaneID: "lane-a", Branch: "feature/lock", Worktree: &wta, Modules: []string{"runtime/mcp"}},
		{LaneID: "lane-b", Branch: "feature/lock", Worktree: &wtb, Modules: []string{"runtime/mcp"}},
	})

	if len(collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(collisions))
	}
	if collisions[0].Branch != "feature/lock" {
		t.Errorf("branch = %s", collisions[0].Branch)
	}
	if collisions[0].Module != "runtime/mcp" {
		t.Errorf("module = %s", collisions[0].Module)
	}
}

func TestDetectsNestedModuleScopeCollisions(t *testing.T) {
	collisions := DetectBranchLockCollisions([]BranchLockIntent{
		{LaneID: "lane-a", Branch: "feature/lock", Modules: []string{"runtime"}},
		{LaneID: "lane-b", Branch: "feature/lock", Modules: []string{"runtime/mcp"}},
	})

	if len(collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(collisions))
	}
	if collisions[0].Module != "runtime" {
		t.Errorf("module = %s, want runtime", collisions[0].Module)
	}
}

func TestIgnoresDifferentBranches(t *testing.T) {
	collisions := DetectBranchLockCollisions([]BranchLockIntent{
		{LaneID: "lane-a", Branch: "feature/a", Modules: []string{"runtime/mcp"}},
		{LaneID: "lane-b", Branch: "feature/b", Modules: []string{"runtime/mcp"}},
	})

	if len(collisions) != 0 {
		t.Fatalf("expected 0 collisions, got %d", len(collisions))
	}
}

func TestBranchLockManagerAcquireAndRelease(t *testing.T) {
	mgr := NewBranchLockManager()

	err := mgr.Acquire(BranchLockIntent{
		LaneID:  "lane-a",
		Branch:  "main",
		Modules: []string{"src/api"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Different module on same branch should be fine
	err = mgr.Acquire(BranchLockIntent{
		LaneID:  "lane-b",
		Branch:  "main",
		Modules: []string{"src/web"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Overlapping module should fail
	err = mgr.Acquire(BranchLockIntent{
		LaneID:  "lane-c",
		Branch:  "main",
		Modules: []string{"src/api/v2"},
	})
	if !errors.Is(err, ErrBranchCollision) {
		t.Fatalf("expected ErrBranchCollision, got %v", err)
	}

	// Release lane-a, then lane-c should succeed
	mgr.Release("lane-a")
	err = mgr.Acquire(BranchLockIntent{
		LaneID:  "lane-c",
		Branch:  "main",
		Modules: []string{"src/api/v2"},
	})
	if err != nil {
		t.Fatalf("expected success after release, got %v", err)
	}

	claims := mgr.ActiveClaims()
	if len(claims) != 2 {
		t.Errorf("expected 2 claims, got %d", len(claims))
	}
}

func TestBranchLockManagerConcurrentSafety(t *testing.T) {
	mgr := NewBranchLockManager()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			laneID := "lane-" + string(rune('a'+id))
			branch := "feature/" + string(rune('a'+id))
			_ = mgr.Acquire(BranchLockIntent{
				LaneID:  laneID,
				Branch:  branch,
				Modules: []string{"src"},
			})
		}(i)
	}
	wg.Wait()

	claims := mgr.ActiveClaims()
	if len(claims) != 20 {
		t.Errorf("expected 20 claims (all different branches), got %d", len(claims))
	}
}
