package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeInstallerCommands struct {
	crashDuringCopy    bool
	limitOnReplacement bool
	rateLimited        bool
	afterInstall       func()
	i                  *Installer
	t                  *testing.T
	old                string
	active             bool
	startFailure       bool
	installFailure     bool
	helperFailure      bool
	crashOnStart       bool
	cancelInstall      context.CancelFunc
	dropIn             bool
	log                []string
}

func (c *fakeInstallerCommands) Run(ctx context.Context, program string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call := filepath.Base(program) + " " + strings.Join(args, " ")
	c.log = append(c.log, call)
	if filepath.Base(program) == "roundd" {
		return []byte(UpdateProtocol + "\n"), nil
	}
	switch args[0] {
	case "show":
		if strings.Contains(call, "--property=FragmentPath") {
			return []byte(filepath.Join(filepath.Dir(c.i.unitFile), args[1]) + "\n"), nil
		}
		if strings.Contains(call, "--property=DropInPaths") {
			if c.dropIn {
				return []byte("override.conf\n"), nil
			}
			return nil, nil
		}
		return []byte("notify\n"), nil
	case "stop":
		c.active = false
	case "start":
		if len(args) > 1 && args[1] == "--no-block" {
			if c.helperFailure {
				return nil, errors.New("cannot start helper")
			}
			return nil, nil
		}
		current, _ := c.i.releaseLink("current", false)
		if c.rateLimited {
			return nil, errors.New("start-limit-hit")
		}
		if current != c.old {
			if c.crashOnStart {
				c.crashOnStart = false
				panic("simulated process crash")
			}
			if c.startFailure {
				if c.limitOnReplacement {
					c.rateLimited = true
				}
				return nil, errors.New("replacement never became ready")
			}
		}
		c.active = true
	case "reset-failed":
		c.rateLimited = false
	case "is-active":
		if !c.active {
			return nil, errors.New("inactive")
		}
	}
	return nil, nil
}

func writeInstalledFixture(t *testing.T, dir, version string) {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range []string{"roundd", "round-clock", "round-timer"} {
		files["bin/"+name] = []byte(name + " " + version)
	}
	for _, name := range managedUpdateUnits() {
		files[name] = []byte("[Unit]\nDescription=" + name + " " + version + "\n[Service]\nType=notify\n")
	}
	for name, data := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), checksumsFor(files), 0644); err != nil {
		t.Fatal(err)
	}
}

func (c *fakeInstallerCommands) Install(ctx context.Context, root, stage string) error {
	c.log = append(c.log, "install")
	if c.crashDuringCopy {
		os.Mkdir(stage, 0700)
		os.WriteFile(filepath.Join(stage, "partial"), []byte("partial"), 0600)
		panic("interrupted installer copy")
	}
	newRelease := filepath.Join(c.i.root, "releases", "0.6.0")
	writeInstalledFixture(c.t, newRelease, "0.6.0")
	if err := atomicInstallLink(c.i.root, "previous", c.old); err != nil {
		return err
	}
	if err := atomicInstallLink(c.i.root, "current", newRelease); err != nil {
		return err
	}
	if c.afterInstall != nil {
		c.afterInstall()
	}
	if c.cancelInstall != nil {
		c.cancelInstall()
		return ctx.Err()
	}
	if c.installFailure {
		return errors.New("installer failed after switching")
	}
	return nil
}

func installerFixture(t *testing.T) (*Installer, *fakeInstallerCommands, Release) {
	t.Helper()
	dir := t.TempDir()
	i := &Installer{root: filepath.Join(dir, "roundos"), stateDir: filepath.Join(dir, "state"), cache: filepath.Join(dir, "cache"), unitFile: filepath.Join(dir, "units", "roundos.service")}
	old := filepath.Join(i.root, "releases", "0.5.0")
	writeInstalledFixture(t, old, "0.5.0")
	if err := os.MkdirAll(filepath.Dir(i.unitFile), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range managedUpdateUnits() {
		data, _ := os.ReadFile(filepath.Join(old, name))
		if err := os.WriteFile(filepath.Join(filepath.Dir(i.unitFile), name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"current", "update-controller"} {
		if err := os.Symlink(old, filepath.Join(i.root, name)); err != nil {
			t.Fatal(err)
		}
	}
	c := &fakeInstallerCommands{i: i, t: t, old: old, active: true}
	i.commands = c
	i.stage = func(ctx context.Context, release Release, cache string) (*Package, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := prepareCache(cache); err != nil {
			return nil, err
		}
		stage, err := os.MkdirTemp(cache, ".download-")
		if err != nil {
			return nil, err
		}
		root := filepath.Join(stage, "package")
		if err := os.MkdirAll(filepath.Join(root, "systemd"), 0700); err != nil {
			return nil, err
		}
		for _, name := range managedUpdateUnits() {
			if err := os.WriteFile(filepath.Join(root, "systemd", name), []byte("[Unit]\nDescription="+name+" "+release.Version+"\n[Service]\nType=notify\n"), 0600); err != nil {
				return nil, err
			}
		}
		return &Package{Root: root, Version: release.Version, Commit: strings.Repeat("a", 40), stage: stage}, nil
	}
	return i, c, Release{ID: 123, Version: "0.6.0", Tag: "v0.6.0"}
}

func assertManagedLink(t *testing.T, i *Installer, name, want string) {
	t.Helper()
	got, err := i.releaseLink(name, false)
	if err != nil || got != want {
		t.Fatalf("%s=%s, want %s: %v", name, got, want, err)
	}
}

func TestInstallerRequestAndSuccessfulIndependentHandoff(t *testing.T) {
	i, c, r := installerFixture(t)
	queued, err := i.Request(context.Background(), r)
	if err != nil || queued.Phase != "queued" {
		t.Fatalf("request: %+v, %v", queued, err)
	}
	if _, err := i.Request(context.Background(), r); !errors.Is(err, ErrInstallBusy) {
		t.Fatal("overlapping request accepted")
	}
	assertManagedLink(t, i, "current", c.old)
	if err := i.RunPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, err := i.ReadStatus()
	if err != nil || st.ID != queued.ID || st.Phase != "succeeded" {
		t.Fatalf("status: %+v, %v", st, err)
	}
	newRelease := filepath.Join(i.root, "releases", r.Version)
	assertManagedLink(t, i, "current", newRelease)
	assertManagedLink(t, i, "previous", c.old)
	assertManagedLink(t, i, "update-controller", newRelease)
	assertEmptyCache(t, i.cache)
	if !c.active {
		t.Fatal("replacement service is inactive")
	}
	count := len(c.log)
	if err := i.RunPending(context.Background()); err != nil || len(c.log) != count {
		t.Fatal("completed request ran again")
	}
}

func TestInstallerFailuresRestoreOldReleaseAndUnits(t *testing.T) {
	for _, failure := range []string{"installer", "readiness", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			i, c, r := installerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch failure {
			case "installer":
				c.installFailure = true
			case "readiness":
				c.startFailure = true
			case "cancellation":
				c.cancelInstall = cancel
			}
			if _, err := i.Request(ctx, r); err != nil {
				t.Fatal(err)
			}
			if err := i.RunPending(ctx); err == nil {
				t.Fatal("failed update reported success")
			}
			st, err := i.ReadStatus()
			if err != nil || st.Phase != "rolled_back" {
				t.Fatalf("rollback: %+v, %v", st, err)
			}
			assertManagedLink(t, i, "current", c.old)
			assertManagedLink(t, i, "update-controller", c.old)
			if !c.active {
				t.Fatal("old service was not restarted")
			}
			for _, name := range managedUpdateUnits() {
				got, _ := os.ReadFile(filepath.Join(filepath.Dir(i.unitFile), name))
				want, _ := os.ReadFile(filepath.Join(c.old, name))
				if string(got) != string(want) {
					t.Fatalf("%s not restored", name)
				}
			}
			if _, err := os.Lstat(filepath.Join(i.root, "releases", r.Version)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed release was not quarantined")
			}
			if _, err := i.Request(context.Background(), r); err != nil {
				t.Fatalf("retry was blocked: %v", err)
			}
		})
	}
}

func TestInstallerPreparationFailuresNeverStopShell(t *testing.T) {
	for _, failure := range []string{"download", "custom unit", "helper launch"} {
		t.Run(failure, func(t *testing.T) {
			i, c, r := installerFixture(t)
			switch failure {
			case "download":
				i.stage = func(context.Context, Release, string) (*Package, error) { return nil, errors.New("checksum mismatch") }
			case "custom unit":
				os.WriteFile(i.unitFile, []byte("custom unit"), 0644)
			case "helper launch":
				c.helperFailure = true
			}
			_, requestErr := i.Request(context.Background(), r)
			if failure == "download" {
				if requestErr != nil {
					t.Fatal(requestErr)
				}
				if i.RunPending(context.Background()) == nil {
					t.Fatal("download failure hidden")
				}
			} else if requestErr == nil {
				t.Fatal("invalid request accepted")
			}
			for _, call := range c.log {
				if strings.Contains(call, "stop") || call == "install" {
					t.Fatal("preparation failure stopped shell")
				}
			}
			assertManagedLink(t, i, "current", c.old)
		})
	}
}

func TestInstallerBootRecoveryRestoresWithoutServiceOrderingDeadlock(t *testing.T) {
	i, c, r := installerFixture(t)
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	c.crashOnStart = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("crash was not injected")
			}
		}()
		i.RunPending(context.Background())
	}()
	st, _ := i.ReadStatus()
	if st.Phase != "starting" {
		t.Fatalf("crash phase: %+v", st)
	}
	c.log = nil
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range c.log {
		if strings.HasPrefix(call, "systemctl start") || strings.HasPrefix(call, "systemctl stop") {
			t.Fatal("boot recovery queued a competing RoundOS start/stop job")
		}
	}
	st, _ = i.ReadStatus()
	if st.Phase != "rolled_back" {
		t.Fatalf("recovery outcome: %+v", st)
	}
	assertManagedLink(t, i, "current", c.old)
}

func TestInstallerRecoveryRejectsCorruptBackup(t *testing.T) {
	i, c, r := installerFixture(t)
	i.Request(context.Background(), r)
	c.crashOnStart = true
	func() { defer func() { recover() }(); i.RunPending(context.Background()) }()
	var transaction installTransaction
	if err := readInstallJSON(i.transactionFile(), &transaction); err != nil {
		t.Fatal(err)
	}
	unit := transaction.OldUnits["roundos.service"]
	unit.Data = []byte("corrupt")
	transaction.OldUnits["roundos.service"] = unit
	if err := writeInstallJSON(i.transactionFile(), transaction); err != nil {
		t.Fatal(err)
	}
	if err := i.Recover(context.Background()); err == nil {
		t.Fatal("corrupt backup was trusted")
	}
	st, _ := i.ReadStatus()
	if st.Phase != "recovery_required" {
		t.Fatalf("missing actionable recovery status: %+v", st)
	}
}

func TestInstallerProcessLockAndInterruptedDownloadCleanup(t *testing.T) {
	i, _, r := installerFixture(t)
	lock, err := i.lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal("boot recovery competed with live installer")
	}
	lock.Close()
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := prepareCache(i.cache); err != nil {
		t.Fatal(err)
	}
	abandoned := filepath.Join(i.cache, ".download-abandoned")
	os.Mkdir(abandoned, 0700)
	os.WriteFile(filepath.Join(abandoned, "partial"), []byte("partial"), 0600)
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abandoned stage survived recovery")
	}
}

func TestInstallerRollbackDoesNotExecuteOrRequireOlderPrevious(t *testing.T) {
	i, c, r := installerFixture(t)
	previous := filepath.Join(i.root, "releases", "0.4.0")
	writeInstalledFixture(t, previous, "0.4.0")
	os.WriteFile(filepath.Join(previous, "bin/round-timer"), []byte("corrupt older file"), 0755)
	if err := atomicInstallLink(i.root, "previous", previous); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	c.startFailure = true
	if err := i.RunPending(context.Background()); err == nil {
		t.Fatal("failed replacement accepted")
	}
	st, _ := i.ReadStatus()
	if st.Phase != "rolled_back" {
		t.Fatalf("unused older binary blocked recovery: %+v", st)
	}
	assertManagedLink(t, i, "current", c.old)
	assertManagedLink(t, i, "previous", previous)
}

func TestInstallerBootRecoveryClearsOnlyKnownScratchLinks(t *testing.T) {
	for _, unexpected := range []bool{false, true} {
		t.Run(map[bool]string{false: "known links", true: "unexpected artifact"}[unexpected], func(t *testing.T) {
			i, c, r := installerFixture(t)
			i.Request(context.Background(), r)
			c.crashOnStart = true
			func() { defer func() { recover() }(); i.RunPending(context.Background()) }()
			for _, name := range []string{".previous.new", ".current.new", ".update-controller.new"} {
				target := c.old
				if name != ".previous.new" {
					target = filepath.Join(i.root, "releases", r.Version)
				}
				if err := os.Symlink(target, filepath.Join(i.root, name)); err != nil {
					t.Fatal(err)
				}
			}
			if unexpected {
				os.Remove(filepath.Join(i.root, ".current.new"))
				os.WriteFile(filepath.Join(i.root, ".current.new"), []byte("retain this file"), 0600)
			}
			err := i.Recover(context.Background())
			if unexpected {
				if err == nil {
					t.Fatal("unexpected file removed")
				}
				data, _ := os.ReadFile(filepath.Join(i.root, ".current.new"))
				if string(data) != "retain this file" {
					t.Fatal("unexpected artifact changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{".previous.new", ".current.new", ".update-controller.new"} {
				if _, err := os.Lstat(filepath.Join(i.root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("scratch link blocked retry")
				}
			}
		})
	}
}

func TestInstallerRejectsEffectiveSystemdOverride(t *testing.T) {
	i, c, r := installerFixture(t)
	c.dropIn = true
	if _, err := i.Request(context.Background(), r); err == nil {
		t.Fatal("effective systemd override accepted")
	}
	assertManagedLink(t, i, "current", c.old)
}

func TestInstallerFlushRejectsLinksAndHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "roundd"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := syncInstalledRelease(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := syncInstalledRelease(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("flush cancellation: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "unexpected")); err != nil {
		t.Fatal(err)
	}
	if err := syncInstalledRelease(context.Background(), dir); err == nil {
		t.Fatal("release flush followed a link")
	}
}

func TestInstallerRollbackRestoresShellWhenStateWritesFail(t *testing.T) {
	i, c, r := installerFixture(t)
	blocked := false
	i.writeState = func(file string, value any) error {
		if blocked {
			return errors.New("state filesystem read-only")
		}
		return writeInstallJSON(file, value)
	}
	c.afterInstall = func() { blocked = true }
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := i.RunPending(context.Background()); err == nil {
		t.Fatal("journal failure hidden")
	}
	assertManagedLink(t, i, "current", c.old)
	if !c.active {
		t.Fatal("unwritable progress journal prevented old shell restart")
	}
	blocked = false
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, _ := i.ReadStatus()
	if st.Phase != "rolled_back" {
		t.Fatalf("durable record did not support recovery replay: %+v", st)
	}
}

func TestInstallerRecoveryOnlyNeedsControllerDaemon(t *testing.T) {
	i, c, r := installerFixture(t)
	controller := filepath.Join(i.root, "releases", "0.4.0")
	writeInstalledFixture(t, controller, "0.4.0")
	os.WriteFile(filepath.Join(controller, "bin/round-clock"), []byte("unused corrupt controller app"), 0755)
	if err := atomicInstallLink(i.root, "update-controller", controller); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	c.startFailure = true
	if err := i.RunPending(context.Background()); err == nil {
		t.Fatal("replacement failure hidden")
	}
	assertManagedLink(t, i, "current", c.old)
	assertManagedLink(t, i, "update-controller", controller)
	if !c.active {
		t.Fatal("unused controller app blocked healthy shell rollback")
	}
}

func TestInstallerUnacknowledgedHelperLaunchReleasesQueuedRequest(t *testing.T) {
	i, c, r := installerFixture(t)
	i.startWait = 10 * time.Millisecond
	st, err := i.Request(context.Background(), r)
	if err == nil || st.Phase != "failed" {
		t.Fatalf("unacknowledged helper remained queued: %+v, %v", st, err)
	}
	if _, err := i.Request(context.Background(), r); errors.Is(err, ErrInstallBusy) {
		t.Fatal("failed helper launch stranded request")
	}
	assertManagedLink(t, i, "current", c.old)
}

func TestInstallerAcceptsVerifiedManagedUnitsAfterManualRollback(t *testing.T) {
	for _, source := range []string{"previous", "update-controller"} {
		t.Run(source, func(t *testing.T) {
			i, _, r := installerFixture(t)
			snapshot := filepath.Join(i.root, "releases", "0.5.1")
			writeInstalledFixture(t, snapshot, "0.5.1")
			if err := atomicInstallLink(i.root, source, snapshot); err != nil {
				t.Fatal(err)
			}
			for _, name := range managedUpdateUnits() {
				data, _ := os.ReadFile(filepath.Join(snapshot, name))
				if err := os.WriteFile(filepath.Join(filepath.Dir(i.unitFile), name), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := i.preflight(context.Background()); err != nil {
				t.Fatalf("unchanged managed units rejected after rollback: %v", err)
			}
			// Matching bytes alone cannot bless a modified snapshot template.
			unit := filepath.Join(snapshot, "roundos.service")
			os.WriteFile(unit, []byte("[Service]\nType=notify\n# modified\n"), 0644)
			os.WriteFile(i.unitFile, []byte("[Service]\nType=notify\n# modified\n"), 0644)
			if _, err := i.Request(context.Background(), r); err == nil {
				t.Fatal("modified managed template was accepted")
			}
		})
	}
}

func TestInstallerWaitsForLifecycleLockAndSettlesQueuedRequest(t *testing.T) {
	i, c, r := installerFixture(t)
	st, err := i.Request(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := i.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	i.startWait = 10 * time.Millisecond
	if err := i.awaitHelperStartup(context.Background(), st.ID); err == nil {
		t.Fatal("unrelated lifecycle lock acknowledged helper startup")
	}
	c.active = false // desktop recovery stopped the managed shell
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- i.RunPending(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("helper exited instead of waiting for lifecycle lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lock.Close()
	if err := <-done; err == nil {
		t.Fatal("inactive managed shell was not reported")
	}
	if status, err := i.ReadStatus(); err != nil || status.Phase != "failed" {
		t.Fatalf("queued request was stranded: %+v, %v", status, err)
	}
	c.active = true
	i.startWait = 0
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatalf("settled request blocked explicit retry: %v", err)
	}
}

func TestInstallerBootRecoveryDoesNotBlockHealthyShellOnOutcomeWriteFailure(t *testing.T) {
	i, c, r := installerFixture(t)
	i.Request(context.Background(), r)
	c.crashOnStart = true
	func() { defer func() { recover() }(); i.RunPending(context.Background()) }()
	i.writeState = func(string, any) error { return errors.New("state filesystem read-only") }
	if err := i.Recover(context.Background()); err != nil {
		t.Fatalf("restored boot was rejected for a journal failure: %v", err)
	}
	assertManagedLink(t, i, "current", c.old)
	if c.active {
		t.Fatal("boot recovery started the dependent unit itself")
	}
}

func TestInstallerRollbackResetsShellStartLimit(t *testing.T) {
	i, c, r := installerFixture(t)
	c.startFailure, c.limitOnReplacement = true, true
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := i.RunPending(context.Background()); err == nil {
		t.Fatal("replacement failure hidden")
	}
	if !c.active || c.rateLimited {
		t.Fatal("start-rate limit blocked the healthy rollback")
	}
	st, _ := i.ReadStatus()
	if st.Phase != "rolled_back" {
		t.Fatalf("rollback outcome: %+v", st)
	}
}

func TestInstallerBootAllowsUnchangedReleaseWhenInterruptedOutcomeCannotPersist(t *testing.T) {
	for _, phase := range []string{"queued", "preparing"} {
		t.Run(phase, func(t *testing.T) {
			i, c, r := installerFixture(t)
			if _, err := i.Request(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			var transaction installTransaction
			if err := readInstallJSON(i.transactionFile(), &transaction); err != nil {
				t.Fatal(err)
			}
			if err := i.phase(&transaction, phase, "preparing"); err != nil {
				t.Fatal(err)
			}
			i.writeState = func(string, any) error { return errors.New("state filesystem read-only") }
			if err := i.Recover(context.Background()); err != nil {
				t.Fatalf("unchanged boot rejected: %v", err)
			}
			assertManagedLink(t, i, "current", c.old)
		})
	}
}

func TestInstallerRecoveryRemovesItsInterruptedReleaseCopy(t *testing.T) {
	i, c, r := installerFixture(t)
	if _, err := i.Request(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	c.crashDuringCopy = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("copy interruption not injected")
			}
		}()
		i.RunPending(context.Background())
	}()
	var transaction installTransaction
	if err := readInstallJSON(i.transactionFile(), &transaction); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(transaction.InstallStage, "partial")); err != nil {
		t.Fatal("partial copy fixture missing")
	}
	if err := i.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(transaction.InstallStage); !os.IsNotExist(err) {
		t.Fatal("interrupted release copy consumed storage after recovery")
	}
	assertManagedLink(t, i, "current", c.old)
}

func TestInstallerDoesNotAutomaticallyReplayRecoveryRequiredTransaction(t *testing.T) {
	i, c, r := installerFixture(t)
	i.Request(context.Background(), r)
	var transaction installTransaction
	readInstallJSON(i.transactionFile(), &transaction)
	if err := i.phase(&transaction, "recovery_required", "SSH required"); err != nil {
		t.Fatal(err)
	}
	before := len(c.log)
	if err := i.RunPending(context.Background()); err != nil || len(c.log) != before {
		t.Fatal("helper restart attempted an unsafe automatic recovery")
	}
}
