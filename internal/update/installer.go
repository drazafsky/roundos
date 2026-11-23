package update

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const UpdateProtocol = "roundos-update-v1"

type InstallStatus struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

func (s InstallStatus) InProgress() bool {
	switch s.Phase {
	case "queued", "preparing", "switching", "starting", "committing", "recovering":
		return true
	}
	return false
}

type installRequest struct {
	ID      string  `json:"id"`
	Release Release `json:"release"`
}

type installTransaction struct {
	InstallStage  string                `json:"install_stage,omitempty"`
	Status        InstallStatus         `json:"status"`
	OldCurrent    string                `json:"old_current"`
	OldPrevious   string                `json:"old_previous"`
	OldController string                `json:"old_controller"`
	OldUnits      map[string]unitBackup `json:"old_units"`
	NewRelease    string                `json:"new_release"`
	Stage         string                `json:"stage"`
	Detail        string                `json:"detail,omitempty"`
}

type unitBackup struct {
	Data   []byte `json:"data"`
	Digest string `json:"digest"`
}

func managedUpdateUnits() []string {
	return []string{"roundos.service", "roundos-update.service", "roundos-update-recover.service"}
}

func validateInstallStatus(status InstallStatus) error {
	if len(status.ID) != 32 || strings.Trim(status.ID, "0123456789abcdef") != "" {
		return errors.New("invalid installation request ID")
	}
	v, err := parseVersion(status.Version)
	if err != nil || v.pre != "" || strings.HasPrefix(status.Version, "v") {
		return errors.New("invalid installation version")
	}
	switch status.Phase {
	case "queued", "preparing", "switching", "starting", "committing", "recovering", "succeeded", "rolled_back", "failed", "recovery_required":
		return nil
	}
	return errors.New("invalid installation phase")
}

func readManagedUnit(file string) ([]byte, error) {
	if err := trustedInstalledPath(file, false); err != nil {
		return nil, err
	}
	info, err := os.Stat(file)
	if err != nil || info.Size() > 64*1024 {
		return nil, errors.New("managed service unit exceeds size limit")
	}
	return os.ReadFile(file)
}

type installerCommands interface {
	Run(context.Context, string, ...string) ([]byte, error)
	Install(context.Context, string, string) error
}

type localInstallerCommands struct{}

type installOutput struct{ buffer bytes.Buffer }

func (o *installOutput) Write(p []byte) (int, error) {
	if len(p) > 16*1024-o.buffer.Len() {
		return 0, errors.New("installer command output exceeds limit")
	}
	return o.buffer.Write(p)
}

func boundedInstallCommand(command *exec.Cmd) ([]byte, error) {
	var out installOutput
	command.Stdout, command.Stderr = &out, &out
	err := runUpdateCommand(command)
	return out.buffer.Bytes(), err
}

func (localInstallerCommands) Run(ctx context.Context, program string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	output, err := boundedInstallCommand(command)
	if err != nil {
		return output, fmt.Errorf("%s: %w: %s", filepath.Base(program), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (localInstallerCommands) Install(ctx context.Context, root, stage string) error {
	command := exec.CommandContext(ctx, "/bin/sh", filepath.Join(root, "scripts/install-release.sh"))
	command.Dir = root
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "ROUNDOS_UPDATE_TRANSACTION=1", "ROUNDOS_UPDATE_STAGE=" + stage}
	output, err := boundedInstallCommand(command)
	if err != nil {
		return fmt.Errorf("managed installer: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Installer is used by both the shell's request worker and a separate systemd
// helper process. Production paths are fixed; optional apps cannot choose paths,
// executables, service names, or installer arguments.
type Installer struct {
	root       string
	stateDir   string
	cache      string
	unitFile   string
	commands   installerCommands
	stage      func(context.Context, Release, string) (*Package, error)
	startWait  time.Duration
	writeState func(string, any) error
}

func NewInstaller() *Installer {
	stager := NewStager()
	return &Installer{root: "/opt/roundos", stateDir: "/var/lib/roundos/shell-data", cache: "/var/lib/roundos/update-cache",
		unitFile: "/etc/systemd/system/roundos.service", commands: localInstallerCommands{}, stage: stager.Stage, startWait: 10 * time.Second}
}

func (i *Installer) transactionFile() string { return filepath.Join(i.stateDir, "update-install.json") }
func (i *Installer) requestFile() string     { return filepath.Join(i.stateDir, "update-request.json") }

func (i *Installer) ReadStatus() (InstallStatus, error) {
	var transaction installTransaction
	err := readInstallJSON(i.transactionFile(), &transaction)
	if errors.Is(err, os.ErrNotExist) {
		return InstallStatus{}, nil
	}
	if err == nil {
		err = validateInstallStatus(transaction.Status)
	}
	return transaction.Status, err
}

// Supported is intended for a background worker, never the render/input loop.
func (i *Installer) Supported(ctx context.Context) error {
	if os.Geteuid() != 0 || runtime.GOOS != "linux" || runtime.GOARCH != "arm" {
		return errors.New("installation requires the managed 32-bit ARM device service")
	}
	_, err := i.preflight(ctx)
	return err
}

func (i *Installer) preflight(ctx context.Context) (string, error) {
	if err := trustedInstalledPath(i.root, true); err != nil {
		return "", err
	}
	if err := trustedInstalledPath(filepath.Join(i.root, "releases"), true); err != nil {
		return "", err
	}
	current, err := i.releaseLink("current", false)
	if err != nil {
		return "", err
	}
	controller, err := i.releaseLink("update-controller", false)
	if err != nil {
		return "", errors.New("managed update controller is not installed")
	}
	for _, dir := range []string{filepath.Join(current, "bin"), filepath.Join(controller, "bin")} {
		if err := trustedInstalledPath(dir, true); err != nil {
			return "", err
		}
	}
	if err := trustedInstalledPath(filepath.Join(current, "SHA256SUMS"), false); err != nil {
		return "", err
	}
	checksums, err := readChecksums(filepath.Join(current, "SHA256SUMS"))
	if err != nil {
		return "", err
	}
	for _, name := range []string{"bin/roundd", "bin/round-clock", "bin/round-timer", "roundos.service"} {
		if err := trustedInstalledPath(filepath.Join(current, name), false); err != nil {
			return "", err
		}
		if err := verifyFile(ctx, filepath.Join(current, name), checksums[name]); err != nil {
			return "", err
		}
	}
	previous, _ := i.releaseLink("previous", true)
	for _, name := range managedUpdateUnits() {
		template, err := readManagedUnit(filepath.Join(current, name))
		if err != nil {
			return "", err
		}
		if err := verifyFile(ctx, filepath.Join(current, name), checksums[name]); err != nil {
			return "", err
		}
		unit, err := readManagedUnit(filepath.Join(filepath.Dir(i.unitFile), name))
		if err != nil || (!bytes.Equal(unit, template) && !matchesManagedTemplate(ctx, unit, name, controller, previous)) {
			return "", errors.New("custom service configuration requires a manual update")
		}
		if err := i.effectiveManagedUnit(ctx, name); err != nil {
			return "", err
		}
	}
	for _, file := range []string{"SHA256SUMS", "bin/roundd"} {
		if err := trustedInstalledPath(filepath.Join(controller, file), false); err != nil {
			return "", err
		}
	}
	controllerChecksums, err := readChecksums(filepath.Join(controller, "SHA256SUMS"))
	if err != nil {
		return "", err
	}
	if err := verifyFile(ctx, filepath.Join(controller, "bin/roundd"), controllerChecksums["bin/roundd"]); err != nil {
		return "", err
	}
	protocol, err := i.commands.Run(ctx, filepath.Join(controller, "bin/roundd"), "-update-protocol")
	if err != nil || string(protocol) != UpdateProtocol+"\n" {
		return "", errors.New("managed update controller is incompatible")
	}
	typeName, err := i.commands.Run(ctx, "/usr/bin/systemctl", "show", "roundos.service", "--property=Type", "--value")
	if err != nil || strings.TrimSpace(string(typeName)) != "notify" {
		return "", errors.New("managed readiness service is required for installation")
	}
	if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "roundos.service"); err != nil {
		return "", errors.New("RoundOS must be running as its managed service")
	}
	return current, nil
}

// Manual rollback keeps the installed units and controller. Accept only an
// unchanged, checksum-verified template from one of those managed snapshots.
func matchesManagedTemplate(ctx context.Context, unit []byte, name string, snapshots ...string) bool {
	for _, snapshot := range snapshots {
		if snapshot == "" || trustedInstalledPath(filepath.Join(snapshot, "SHA256SUMS"), false) != nil {
			continue
		}
		checksums, err := readChecksums(filepath.Join(snapshot, "SHA256SUMS"))
		if err != nil {
			continue
		}
		file := filepath.Join(snapshot, name)
		template, err := readManagedUnit(file)
		if err == nil && bytes.Equal(unit, template) && verifyFile(ctx, file, checksums[name]) == nil {
			return true
		}
	}
	return false
}

func (i *Installer) Request(ctx context.Context, release Release) (InstallStatus, error) {
	lock, err := i.lock()
	if err != nil {
		return InstallStatus{}, err
	}
	defer lock.Close()
	status, err := i.ReadStatus()
	if err != nil {
		return status, err
	}
	if status.InProgress() || status.Phase == "recovery_required" {
		return status, ErrInstallBusy
	}
	current, err := i.preflight(ctx)
	if err != nil {
		return status, err
	}
	candidate, candidateErr := parseVersion(release.Version)
	installed, installedErr := parseVersion(filepath.Base(current))
	if candidateErr != nil || installedErr != nil || candidate.pre != "" || !newerStable(candidate, installed) || release.ID <= 0 || strings.HasPrefix(release.Version, "v") {
		return status, errors.New("selected stable release is not newer than the installed version")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return status, err
	}
	id := hex.EncodeToString(nonce[:])
	release.Notes, release.Name = "", "" // notes are not part of the privileged handoff
	request := installRequest{ID: id, Release: release}
	transaction := installTransaction{Status: InstallStatus{ID: id, Version: release.Version, Phase: "queued", Message: "Preparing to install the selected release"}}
	if err := writeInstallJSON(i.requestFile(), request); err != nil {
		return status, err
	}
	if err := writeInstallJSON(i.transactionFile(), transaction); err != nil {
		return status, err
	}
	lock.Close() // the helper must acquire the lock before it can start staging
	_, err = i.commands.Run(ctx, "/usr/bin/systemctl", "start", "--no-block", "roundos-update.service")
	if err == nil && i.startWait > 0 {
		err = i.awaitHelperStartup(ctx, id)
	}
	if err != nil {
		// If starting the job definitively failed, release the queued request
		// so a later user retry can succeed. Never overwrite a helper that
		// already acquired the lock and started processing this request.
		if failureLock, lockErr := i.lock(); lockErr == nil {
			var latest installTransaction
			if readInstallJSON(i.transactionFile(), &latest) == nil && latest.Status.ID == id && latest.Status.Phase == "queued" {
				err = i.fail(&latest, err)
				transaction = latest
			}
			failureLock.Close()
		}
		return transaction.Status, err
	}
	if latest, err := i.ReadStatus(); err == nil && latest.ID == id {
		return latest, nil
	}
	return transaction.Status, nil
}

func (i *Installer) awaitHelperStartup(ctx context.Context, id string) error {
	timer := time.NewTimer(i.startWait)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if status, err := i.ReadStatus(); err != nil {
			return err
		} else if status.ID != id {
			return errors.New("update helper request was replaced before acknowledgement")
		} else if status.Phase != "queued" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-timer.C:
			return errors.New("update helper did not acknowledge startup")
		}
	}
}

func (i *Installer) RunPending(ctx context.Context) error {
	// A lifecycle operation can take the lock between Request and helper
	// startup. Wait for it rather than exhausting systemd's short retry burst
	// and leaving a durable queued request without a worker.
	lock, err := i.awaitInstallLock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	var transaction installTransaction
	if err := readInstallJSON(i.transactionFile(), &transaction); err != nil {
		return err
	}
	if err := validateInstallStatus(transaction.Status); err != nil {
		return err
	}
	if transaction.Status.Phase != "queued" {
		if transaction.Status.InProgress() {
			return i.recoverLocked(ctx, &transaction, true)
		}
		return nil // a failed/prepared request is never installed again implicitly
	}
	var request installRequest
	if err := readInstallJSON(i.requestFile(), &request); err != nil {
		return i.fail(&transaction, err)
	}
	if request.ID != transaction.Status.ID || len(request.ID) != 32 || request.Release.Version != transaction.Status.Version {
		return i.fail(&transaction, errors.New("installation request identity mismatch"))
	}
	current, err := i.preflight(ctx)
	if err != nil {
		return i.fail(&transaction, err)
	}
	installed, err := parseVersion(filepath.Base(current))
	if err != nil {
		return i.fail(&transaction, err)
	}
	candidate, err := parseVersion(request.Release.Version)
	if err != nil || candidate.pre != "" || !newerStable(candidate, installed) {
		return i.fail(&transaction, errors.New("selected release is not newer than the installed version"))
	}
	transaction.NewRelease = filepath.Join(i.root, "releases", request.Release.Version)
	if _, err := os.Lstat(transaction.NewRelease); !errors.Is(err, os.ErrNotExist) {
		return i.fail(&transaction, errors.New("selected release directory already exists"))
	}
	if err := i.phase(&transaction, "preparing", "Downloading and verifying the update"); err != nil {
		return err
	}
	p, err := i.stage(ctx, request.Release, i.cache)
	if err != nil {
		return i.fail(&transaction, err)
	}
	defer p.Cleanup()
	protocol, err := i.commands.Run(ctx, filepath.Join(p.Root, "roundd"), "-update-protocol")
	if err != nil || string(protocol) != UpdateProtocol+"\n" {
		return i.fail(&transaction, errors.New("selected release does not support managed updates"))
	}
	for _, name := range managedUpdateUnits() {
		if _, err := readManagedUnit(filepath.Join(p.Root, "systemd", name)); err != nil {
			return i.fail(&transaction, err)
		}
	}
	// Repeat preflight after a potentially long download so an administrator's
	// intervening service/template changes are never silently overwritten.
	if latest, err := i.preflight(ctx); err != nil || latest != current {
		return i.fail(&transaction, errors.New("managed installation changed during preparation"))
	}
	transaction.OldCurrent = current
	transaction.OldPrevious, err = i.releaseLink("previous", true)
	if err != nil {
		return i.fail(&transaction, err)
	}
	transaction.OldController, err = i.releaseLink("update-controller", false)
	if err != nil {
		return i.fail(&transaction, err)
	}
	transaction.OldUnits = make(map[string]unitBackup)
	for _, name := range managedUpdateUnits() {
		data, err := readManagedUnit(filepath.Join(filepath.Dir(i.unitFile), name))
		if err != nil {
			return i.fail(&transaction, err)
		}
		transaction.OldUnits[name] = unitBackup{Data: data, Digest: unitDigest(data)}
	}
	transaction.Stage = p.stage
	transaction.InstallStage = filepath.Join(i.root, "releases", ".install-update-"+transaction.Status.ID)
	if _, err := os.Lstat(transaction.InstallStage); !errors.Is(err, os.ErrNotExist) {
		return i.fail(&transaction, errors.New("installer staging directory already exists"))
	}
	if err := i.phase(&transaction, "switching", "Installing the update; RoundOS will restart"); err != nil {
		return err
	}
	if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "stop", "roundos.service"); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := i.commands.Install(ctx, p.Root, transaction.InstallStage); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	// The recovery controller must never point at data that exists only in the
	// page cache. Flush files and directory entries before readiness or commit.
	if err := syncInstalledRelease(ctx, transaction.NewRelease); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	unit, err := os.ReadFile(filepath.Join(p.Root, "systemd", "roundos.service"))
	if err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := replaceUnit(i.unitFile, unit); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "daemon-reload"); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := i.phase(&transaction, "starting", "Starting the new version"); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := i.startReady(ctx); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := i.phase(&transaction, "committing", "Finishing the update"); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	if err := i.commitController(ctx, &transaction, p); err != nil {
		return i.rollback(ctx, &transaction, err)
	}
	return i.phase(&transaction, "succeeded", "Update installed successfully")
}

func (i *Installer) phase(transaction *installTransaction, phase, message string) error {
	transaction.Status.Phase = phase
	transaction.Status.Message = message
	if len(transaction.Detail) > 8192 {
		transaction.Detail = transaction.Detail[:8192]
	}
	write := i.writeState
	if write == nil {
		write = writeInstallJSON
	}
	return write(i.transactionFile(), transaction)
}

func (i *Installer) fail(transaction *installTransaction, cause error) error {
	transaction.Detail = cause.Error()
	err := i.phase(transaction, "failed", "Update preparation failed; the installed version is unchanged")
	return errors.Join(cause, err)
}

func (i *Installer) startReady(ctx context.Context) error {
	typeName, err := i.commands.Run(ctx, "/usr/bin/systemctl", "show", "roundos.service", "--property=Type", "--value")
	if err != nil || strings.TrimSpace(string(typeName)) != "notify" {
		return errors.New("replacement service lacks managed readiness")
	}
	if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "start", "roundos.service"); err != nil {
		return err
	}
	_, err = i.commands.Run(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "roundos.service")
	return err
}

func (i *Installer) rollback(ctx context.Context, transaction *installTransaction, cause error) error {
	transaction.Detail = cause.Error()
	if errors.Is(cause, errUpdateChildrenActive) {
		return errors.Join(cause, i.phase(transaction, "recovery_required", "Update processes need attention; inspect the helper over SSH"))
	}
	// An expired download/install context must not prevent restoring the old
	// shell. Recovery has its own bounded window within the helper unit timeout.
	recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return errors.Join(cause, i.recoverLocked(recovery, transaction, true))
}

// Recover runs before roundos.service during boot. If an active installer holds
// the lock, its deliberate service restart proceeds without a competing repair.
func (i *Installer) Recover(ctx context.Context) error {
	if _, err := os.Lstat(i.transactionFile()); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	lock, err := i.lock()
	if errors.Is(err, ErrInstallBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := i.cleanupStages(); err != nil {
		fmt.Fprintf(os.Stderr, "RoundOS recovery could not clean abandoned downloads: %v\n", err)
	}
	var transaction installTransaction
	if err := readInstallJSON(i.transactionFile(), &transaction); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateInstallStatus(transaction.Status); err != nil {
		return err
	}
	if !transaction.Status.InProgress() && transaction.Status.Phase != "recovery_required" {
		return nil
	}
	return i.recoverLocked(ctx, &transaction, false)
}

func (i *Installer) recoverLocked(ctx context.Context, transaction *installTransaction, restart bool) (recoveryErr error) {
	if transaction.Status.Phase == "queued" || transaction.Status.Phase == "preparing" {
		err := i.phase(transaction, "failed", "Update preparation was interrupted; the installed version is unchanged")
		if err != nil && !restart {
			fmt.Fprintf(os.Stderr, "RoundOS installation is unchanged; interrupted preparation outcome persistence failed: %v\n", err)
			return nil
		}
		return err
	}
	defer func() {
		if recoveryErr != nil && transaction.Status.Phase != "recovery_required" {
			transaction.Detail += "\nRecovery: " + recoveryErr.Error()
			recoveryErr = errors.Join(recoveryErr, i.phase(transaction, "recovery_required", "Recovery needs attention; connect over SSH and inspect the update log"))
		}
	}()
	if !i.releasePath(transaction.OldCurrent) || !i.releasePath(transaction.OldController) ||
		(transaction.OldPrevious != "" && !i.releasePath(transaction.OldPrevious)) ||
		!i.releasePath(transaction.NewRelease) || len(transaction.OldUnits) != len(managedUpdateUnits()) {
		return errors.New("invalid installation recovery record; use SSH recovery")
	}
	for _, name := range managedUpdateUnits() {
		unit := transaction.OldUnits[name]
		if len(unit.Data) == 0 || unitDigest(unit.Data) != unit.Digest {
			return errors.New("invalid service-unit recovery backup")
		}
	}
	if transaction.OldPrevious != "" {
		if err := trustedInstalledPath(transaction.OldPrevious, true); err != nil {
			return err
		}
	}
	for _, release := range []string{transaction.OldCurrent, transaction.OldController} {
		if release == "" {
			continue
		}
		if err := trustedInstalledPath(release, true); err != nil {
			return err
		}
		if err := trustedInstalledPath(filepath.Join(release, "bin"), true); err != nil {
			return err
		}
		if err := trustedInstalledPath(filepath.Join(release, "SHA256SUMS"), false); err != nil {
			return err
		}
		checksums, err := readChecksums(filepath.Join(release, "SHA256SUMS"))
		if err != nil {
			return err
		}
		names := []string{"bin/roundd"}
		if release == transaction.OldCurrent {
			names = append(names, "bin/round-clock", "bin/round-timer")
		}
		for _, name := range names {
			if err := trustedInstalledPath(filepath.Join(release, name), false); err != nil {
				return err
			}
			if err := verifyFile(ctx, filepath.Join(release, name), checksums[name]); err != nil {
				return err
			}
		}
	}
	if err := i.phase(transaction, "recovering", "Restoring the previous release"); err != nil {
		// The existing durable switching record already permits replay. A
		// progress-write failure must not prevent restoring a healthy shell.
		transaction.Detail += "\nRecovery progress: " + err.Error()
	}
	restore := func() error {
		if restart {
			if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "stop", "roundos.service"); err != nil {
				return err
			}
		}
		if err := atomicInstallLink(i.root, "current", transaction.OldCurrent); err != nil {
			return err
		}
		if err := atomicInstallLink(i.root, "update-controller", transaction.OldController); err != nil {
			return err
		}
		if transaction.OldPrevious != "" {
			if err := atomicInstallLink(i.root, "previous", transaction.OldPrevious); err != nil {
				return err
			}
		} else if info, err := os.Lstat(filepath.Join(i.root, "previous")); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return errors.New("refusing to remove an untrusted previous-release entry")
			}
			previous, err := filepath.EvalSymlinks(filepath.Join(i.root, "previous"))
			if err != nil || previous != transaction.OldCurrent {
				return errors.New("refusing to remove an unexpected previous release link")
			}
			if err := os.Remove(filepath.Join(i.root, "previous")); err != nil {
				return err
			}
		}
		for _, name := range managedUpdateUnits() {
			if err := replaceUnit(filepath.Join(filepath.Dir(i.unitFile), name), transaction.OldUnits[name].Data); err != nil {
				return err
			}
		}
		if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err := i.commands.Run(ctx, "/usr/bin/systemctl", "reset-failed", "roundos.service"); err != nil {
			return err
		}
		if restart {
			if err := i.startReady(ctx); err != nil {
				return err
			}
		}
		if info, err := os.Lstat(transaction.NewRelease); err == nil {
			if !info.IsDir() || transaction.NewRelease == transaction.OldCurrent || transaction.NewRelease == transaction.OldController || transaction.NewRelease == transaction.OldPrevious {
				return errors.New("unsafe failed release directory")
			}
			quarantine := filepath.Join(i.root, "releases", ".failed-"+filepath.Base(transaction.NewRelease)+"-"+transaction.Status.ID)
			if err := os.Rename(transaction.NewRelease, quarantine); err != nil {
				return err
			}
		}
		if err := i.clearInterruptedLinks(transaction); err != nil {
			return err
		}
		if err := i.clearInstallerStage(transaction); err != nil {
			return err
		}
		return syncStateDirectory(filepath.Join(i.root, "releases"))
	}
	if err := restore(); err != nil {
		transaction.Detail += "\nRecovery: " + err.Error()
		return errors.Join(err, i.phase(transaction, "recovery_required", "Recovery needs attention; connect over SSH and inspect the update log"))
	}
	err := i.phase(transaction, "rolled_back", "The previous release has been restored")
	if err != nil && !restart {
		// Files and units are restored and durable; refusing the boot unit
		// here would prevent PID 1 from starting the healthy restored shell.
		fmt.Fprintf(os.Stderr, "RoundOS recovery restored the previous release; outcome persistence failed: %v\n", err)
		return nil
	}
	return err
}

func (i *Installer) clearInstallerStage(transaction *installTransaction) error {
	stage := transaction.InstallStage
	if stage == "" {
		return nil
	}
	if stage != filepath.Join(i.root, "releases", ".install-update-"+transaction.Status.ID) {
		return errors.New("untrusted installer staging recovery path")
	}
	if _, err := os.Lstat(stage); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := trustedInstalledPath(stage, true); err != nil {
		return err
	}
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	return syncStateDirectory(filepath.Dir(stage))
}

func (i *Installer) effectiveManagedUnit(ctx context.Context, name string) error {
	fragment, err := i.commands.Run(ctx, "/usr/bin/systemctl", "show", name, "--property=FragmentPath", "--value")
	if err != nil || strings.TrimSpace(string(fragment)) != filepath.Join(filepath.Dir(i.unitFile), name) {
		return errors.New("unexpected systemd unit source requires a manual update")
	}
	dropIns, err := i.commands.Run(ctx, "/usr/bin/systemctl", "show", name, "--property=DropInPaths", "--value")
	if err != nil || strings.TrimSpace(string(dropIns)) != "" {
		return errors.New("systemd overrides require a manual update")
	}
	return nil
}

func (i *Installer) clearInterruptedLinks(transaction *installTransaction) error {
	for _, name := range []string{".previous.new", ".current.new", ".update-controller.new"} {
		file := filepath.Join(i.root, name)
		info, err := os.Lstat(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return errors.New("unexpected interrupted installation artifact")
		}
		target, err := os.Readlink(file)
		if err != nil || (target != transaction.OldCurrent && target != transaction.NewRelease && target != transaction.OldController) {
			return errors.New("untrusted interrupted installation link")
		}
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return syncStateDirectory(i.root)
}

func (i *Installer) commitController(ctx context.Context, transaction *installTransaction, p *Package) error {
	// Keep the previous controller until the replacement shell has reported
	// readiness and the recoverable committing phase is durably recorded.
	if err := atomicInstallLink(i.root, "update-controller", transaction.NewRelease); err != nil {
		return err
	}
	for _, name := range []string{"roundos-update.service", "roundos-update-recover.service"} {
		data, err := os.ReadFile(filepath.Join(p.Root, "systemd", name))
		if err != nil {
			return err
		}
		if err := replaceUnit(filepath.Join(filepath.Dir(i.unitFile), name), data); err != nil {
			return err
		}
	}
	_, err := i.commands.Run(ctx, "/usr/bin/systemctl", "daemon-reload")
	return err
}

// Only this installer uses the private cache, and callers hold its process lock.
// Remove abandoned downloads after a crash without following unexpected links.
func (i *Installer) cleanupStages() error {
	if _, err := os.Lstat(i.cache); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := prepareCache(i.cache); err != nil {
		return err
	}
	entries, err := os.ReadDir(i.cache)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".download-") {
			continue
		}
		dir := filepath.Join(i.cache, entry.Name())
		if err := trustedInstalledPath(dir, true); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return syncStateDirectory(i.cache)
}

// Bound the helper independently of service-manager start/stop timeouts.
func InstallContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 9*time.Minute)
}
