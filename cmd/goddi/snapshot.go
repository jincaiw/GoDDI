package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jincaiw/GoDDI/internal/backup"
	"github.com/jincaiw/GoDDI/internal/config"
	applog "github.com/jincaiw/GoDDI/internal/log"
)

// rest holds the flags of the restore subcommand. They are read together
// because they constrain one another: --in-place and --into are alternatives,
// and --in-place means nothing without --yes.
type rest struct {
	input      string
	into       string
	verifyOnly bool
	replace    bool
	inPlace    bool
	yes        bool
}

// snapshotTargets names every database this deployment owns.
//
// All three are listed whatever role the process runs, and each data-plane
// store is optional: a host where the DNS plane has never run has no DNS store,
// and a backup that failed for that reason would fail on exactly the hosts with
// nothing to lose. The absence is recorded in the manifest instead, so "there
// was nothing there" and "the archiver left it out" stay distinguishable.
func snapshotTargets(cfg *config.Config) []backup.SnapshotTarget {
	return []backup.SnapshotTarget{
		{Name: "control", DSN: cfg.Database.DSN, HoldsSecrets: true},
		{Name: "dnsdata", DSN: cfg.DataPlaneDSN(config.DataPlaneZone), Optional: true},
		{Name: "leases", DSN: cfg.DataPlaneDSN(config.DataPlaneLease), Optional: true},
	}
}

// runSnapshot writes a restorable archive of everything this deployment owns.
func runSnapshot(configPath, output string, includes []string, plaintext bool) error {
	if strings.TrimSpace(output) == "" {
		return errors.New("--output is required: a snapshot with nowhere to go would have to be discarded")
	}
	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	// Snapshot source access has not been proven side-effect-free for Enterprise.
	if err := rejectEnterprise(cfg); err != nil {
		return err
	}
	applog.InitLogger(cfg.Log.Level)

	// config.Validate is deliberately not called. A backup is most needed when
	// the deployment is already unhealthy, and refusing to take one because an
	// unrelated path in the configuration is wrong is the wrong way round.
	material := securityKeyMaterial(cfg)
	if plaintext {
		if material != "" {
			fmt.Println("注意：已指定 --plaintext；尽管配置中设置了 encryption_key，该归档仍未加密")
		}
		material = ""
	}

	sources := []backup.ConfigSource{{Path: configPath}}
	for _, include := range includes {
		sources = append(sources, backup.ConfigSource{Path: include, Optional: true})
	}

	manifest, err := backup.CreateSnapshot(backup.SnapshotRequest{
		OutputPath:     output,
		Version:        Version,
		Description:    fmt.Sprintf("%s 快照，角色 %s", cfg.Server.Name, cfg.EffectiveRole()),
		Targets:        snapshotTargets(cfg),
		ConfigFiles:    sources,
		KeyMaterial:    material,
		AllowPlaintext: plaintext,
	})
	if err != nil {
		return err
	}
	describeSnapshot(*manifest, output)
	return nil
}

// runRestore verifies an archive and then does one of three things with it.
func runRestore(configPath string, opts rest) error {
	if strings.TrimSpace(opts.input) == "" {
		return errors.New("--input is required")
	}
	if opts.inPlace && strings.TrimSpace(opts.into) != "" {
		return errors.New("--in-place and --into are alternatives; choose one")
	}
	if !opts.inPlace && !opts.verifyOnly && strings.TrimSpace(opts.into) == "" {
		return errors.New("choose what to do with the archive: --verify to check it, --into <dir> to unpack it, or --in-place to replace this deployment's databases")
	}
	if opts.inPlace && !opts.yes {
		return errors.New("--in-place replaces the databases this deployment serves from and the service must be stopped first; re-run with --yes once it is")
	}

	cfg, err := loadRuntimeConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.ApplyEnvOverrides(cfg)
	// Conservatively reject verify as well until the whole path is reviewed.
	if err := rejectEnterprise(cfg); err != nil {
		return err
	}
	applog.InitLogger(cfg.Log.Level)
	material := securityKeyMaterial(cfg)

	if opts.verifyOnly {
		manifest, err := backup.InspectSnapshot(opts.input, material)
		if err != nil {
			return err
		}
		describeSnapshot(*manifest, opts.input)
		if mismatched := backup.SnapshotKeyMismatches(*manifest, material); len(mismatched) > 0 {
			return keyMismatchError(mismatched)
		}
		// The fit is reported but not enforced here. --verify answers "is this
		// archive intact", which is a question about the artifact; whether
		// this binary can use it is a question about the target, and it is
		// enforced on the paths that write something.
		fitness, err := assessArchive(*manifest)
		if err != nil {
			return err
		}
		describeArchiveFitness(fitness)
		fmt.Println("校验通过：归档完整，且与本机密钥材料一致。未写入任何文件。")
		return nil
	}

	// An in-place restore unpacks into a temporary directory first. Nothing
	// this deployment serves from is touched until the whole archive has been
	// read and checked.
	extractTo := opts.into
	if opts.inPlace {
		staging, err := os.MkdirTemp("", "goddi-restore-")
		if err != nil {
			return fmt.Errorf("创建临时解包目录：%w", err)
		}
		defer os.RemoveAll(staging)
		extractTo = filepath.Join(staging, "archive")
	}

	result, err := backup.ReadSnapshot(backup.SnapshotReadRequest{
		Path:        opts.input,
		KeyMaterial: material,
		ExtractTo:   extractTo,
		Replace:     opts.replace,
	})
	if err != nil {
		return err
	}
	describeSnapshot(result.Manifest, opts.input)
	if mismatched := backup.SnapshotKeyMismatches(result.Manifest, material); len(mismatched) > 0 {
		return keyMismatchError(mismatched)
	}

	// The gate runs before anything is written and on both writing paths, so
	// an archive this binary cannot read is refused on a host that has no
	// databases yet -- the host a restore is prepared on -- and not only on
	// one where a live database happened to be there to compare against.
	fitness, err := assessArchive(result.Manifest)
	if err != nil {
		return err
	}
	describeArchiveFitness(fitness)
	if err := fitness.refusal(); err != nil {
		if !opts.inPlace {
			// The unpack path reads and extracts in one pass, so by the time
			// the gate can judge the manifest the files are already in the
			// directory the operator named. Saying so is better than a bare
			// refusal that leaves them wondering what landed.
			return fmt.Errorf("%w（归档已解包到 %s 以便排查，内容未被改动）", err, result.ExtractedTo)
		}
		return err
	}

	if !opts.inPlace {
		fmt.Printf("已解包到 %s\n", result.ExtractedTo)
		fmt.Println("把它变成一个实例还差三步：把配置指向解包目录、启动服务让迁移把 schema 补齐、再确认密钥材料是写入该归档时的那一份。")
		return nil
	}
	return restoreInPlace(cfg, configPath, result, material)
}

func keyMismatchError(labels []string) error {
	return fmt.Errorf("归档记录的密钥指纹与本机密钥材料不一致（%s）；请用写入该归档时的密钥材料重试",
		strings.Join(labels, ", "))
}

// archiveFitness is what the running binary can make of an archive.
//
// It is a separate value from the manifest because it describes a relationship
// rather than a fact about the file: the same archive is usable by one binary
// and not by another. Reporting the archive and judging its fit are kept apart
// so `--verify` can describe an archive it would refuse to restore.
type archiveFitness struct {
	// ArchiveVersion is the product version the manifest says wrote it.
	ArchiveVersion string

	// VersionKnown is false when either version is not dotted-numeric, which
	// is the case for a development build. The product comparison is then
	// skipped and said out loud, rather than being treated as agreement.
	VersionKnown bool

	// ProductNewer means the archive was written by a newer build than this
	// one.
	ProductNewer bool

	// ProductOlder means the archive is older, which is the ordinary case.
	ProductOlder bool

	// SchemaAhead names stores whose schema in the archive is newer than this
	// binary ships. This is the precise signal: a database migrated further
	// than the binary understands cannot be read by it.
	SchemaAhead []string

	// SchemaBehind names stores the archive is behind on, which the next
	// service start brings forward.
	SchemaBehind []string

	// UnknownStores names stores in the archive this binary has no migrations
	// for. It cannot be judged, and an unjudgeable store is refused rather
	// than assumed readable.
	UnknownStores []string
}

// assessArchive judges an archive against the running binary.
//
// The comparison is against the migrations this binary carries, not against
// the databases this host is currently serving from. The live comparison that
// used to be here was wrong in both directions: it refused the ordinary case
// of restoring last night's backup over a database that had since been
// migrated further, and on a host with no database yet -- which is exactly the
// host a restore is prepared on -- it checked nothing at all.
func assessArchive(manifest backup.SnapshotManifest) (archiveFitness, error) {
	fitness := archiveFitness{ArchiveVersion: manifest.Version}

	if order, ok := compareVersion(manifest.Version, Version); ok {
		fitness.VersionKnown = true
		fitness.ProductNewer = order > 0
		fitness.ProductOlder = order < 0
	}

	ceiling, err := knownSchemaVersions()
	if err != nil {
		return archiveFitness{}, err
	}
	for _, entry := range manifest.Databases {
		known, ok := ceiling[entry.Name]
		if !ok {
			fitness.UnknownStores = append(fitness.UnknownStores, entry.Name)
			continue
		}
		switch {
		case entry.SchemaVersion > known:
			fitness.SchemaAhead = append(fitness.SchemaAhead,
				fmt.Sprintf("%s（归档 %d，本二进制最高 %d）", entry.Name, entry.SchemaVersion, known))
		case entry.SchemaVersion < known:
			fitness.SchemaBehind = append(fitness.SchemaBehind,
				fmt.Sprintf("%s（归档 %d，本二进制最高 %d）", entry.Name, entry.SchemaVersion, known))
		}
	}
	return fitness, nil
}

// refusal is the error this archive deserves, or nil when this binary can use
// it.
//
// Order matters: the schema is the precise signal, so it is reported before
// the product version. The product version is the one a human reads, and it
// would be unhelpful to lead with it when the schema has already said exactly
// which store is too new.
func (f archiveFitness) refusal() error {
	switch {
	case len(f.SchemaAhead) > 0:
		return fmt.Errorf("归档里有存储的 schema 比本二进制自带的更新（%s）；更旧的二进制读不了更新的 schema，请用写入该归档的同一版本或更新的二进制恢复",
			strings.Join(f.SchemaAhead, "、"))
	case len(f.UnknownStores) > 0:
		return fmt.Errorf("归档含有本二进制没有迁移的存储（%s），无法判断它能否被读取；在无法判断时拒绝继续",
			strings.Join(f.UnknownStores, "、"))
	case f.ProductNewer:
		return fmt.Errorf("归档由 GoDDI %s 写入，本二进制是 %s；请用写入该归档的同一版本或更新的二进制恢复",
			f.ArchiveVersion, Version)
	}
	return nil
}

// describeArchiveFitness states the fit in the report, including when it could
// not be established.
func describeArchiveFitness(fitness archiveFitness) {
	if !fitness.VersionKnown {
		fmt.Printf("版本比较已跳过：归档记录为 %q，本二进制为 %q，其中一个不是点分数字版本。是否可用以 schema 版本为准。\n",
			fitness.ArchiveVersion, Version)
	} else if fitness.ProductOlder {
		fmt.Printf("归档由 GoDDI %s 写入，本二进制是 %s（更旧）。\n", fitness.ArchiveVersion, Version)
	}
	if len(fitness.SchemaBehind) > 0 {
		fmt.Printf("归档的 schema 落后于本二进制：%s。下一次启动服务时迁移会把它前移。\n",
			strings.Join(fitness.SchemaBehind, "、"))
	}
	if len(fitness.SchemaAhead) == 0 && len(fitness.UnknownStores) == 0 && !fitness.ProductNewer {
		fmt.Println("本二进制可以读取该归档。")
	}
}

// compareVersion orders two dotted numeric versions.
//
// It reports false when either side is not dotted numeric -- a build stamped
// "dev", or a version carrying a commit suffix. An unreadable version must be
// visible as "compared nothing" rather than silently treated as agreement:
// the caller says so in the report, and the schema comparison is what carries
// the decision.
func compareVersion(left, right string) (int, bool) {
	a, ok := parseVersion(left)
	if !ok {
		return 0, false
	}
	b, ok := parseVersion(right)
	if !ok {
		return 0, false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		switch {
		case x < y:
			return -1, true
		case x > y:
			return 1, true
		}
	}
	return 0, true
}

// parseVersion reads a dotted run of numbers. It tolerates a leading "v",
// which is how release tags spell the same version, and refuses anything else
// rather than reading the numeric prefix and guessing at the rest.
func parseVersion(value string) ([]int, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	if value == "" {
		return nil, false
	}
	fields := strings.Split(value, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			return nil, false
		}
		for _, char := range field {
			if char < '0' || char > '9' {
				return nil, false
			}
		}
		number, err := strconv.Atoi(field)
		if err != nil {
			return nil, false
		}
		parts = append(parts, number)
	}
	return parts, true
}

// restoreInPlace replaces this deployment's databases with the ones in an
// archive.
//
// It is deliberately narrow. It replaces database files, which is the part an
// operator cannot do by hand without knowing where every store lives. It does
// not apply the configuration files from the archive, because those describe
// the host the archive came from and writing them here would silently move this
// instance's data directory. It does not run migrations either: the restored
// schema is brought forward by the next service start, which is the same path
// an upgrade takes and therefore the one that is actually exercised.
//
// Everything that can be checked is checked before anything is written, and a
// consistent copy of the current databases is taken first when there is
// anything to copy, so the one failure mode that remains -- the disk filling up
// halfway through -- still has a way back that is stated in the error.
func restoreInPlace(cfg *config.Config, configPath string, result *backup.SnapshotReadResult, material string) error {
	if len(result.Manifest.Databases) == 0 {
		return errors.New("该归档不含任何数据库，无法就地恢复")
	}
	dataDir := strings.TrimSpace(cfg.Server.DataDir)
	if dataDir == "" {
		return errors.New("server.data_dir 未配置，无法确定替换前副本的存放位置")
	}

	// Empty when there was nothing to preserve, which is why the failure
	// message below says so rather than pointing at a file that was never
	// written.
	safety := ""

	// A safety copy exists to preserve what is about to be replaced, so it is
	// taken only when there is something to preserve.
	//
	// This is the "recover onto a new instance" case, which is the reason an
	// archive exists at all. The copy used to be unconditional, and because
	// snapshotTargets marks the control database as mandatory, --in-place on a
	// host that had never run failed before replacing anything -- reporting a
	// missing database the operator never had. The fresh host is where the
	// command matters most and it was the one place it could not run.
	if existing := existingStores(cfg); len(existing) == 0 {
		fmt.Println("目标主机上没有任何库，没有需要留存的替换前副本")
	} else {
		safety = filepath.Join(dataDir, "backups", "prerestore_"+time.Now().Format("20060102_150405")+backup.SnapshotFileExt)
		if _, err := backup.CreateSnapshot(backup.SnapshotRequest{
			OutputPath:     safety,
			Version:        Version,
			Description:    "就地恢复前的自动副本",
			Targets:        existing,
			ConfigFiles:    []backup.ConfigSource{{Path: configPath, Optional: true}},
			KeyMaterial:    material,
			AllowPlaintext: material == "",
		}); err != nil {
			return fmt.Errorf("替换前复制当前数据库失败，已放弃恢复：%w", err)
		}
		fmt.Printf("替换前已留存当前数据库副本：%s\n", safety)
	}

	locations := map[string]string{}
	for _, target := range snapshotTargets(cfg) {
		locations[target.Name] = backup.DSNPath(target.DSN)
	}

	type step struct {
		name  string
		live  string
		image string

		// The two schema versions are carried so the swap is reported with
		// both numbers. Whether this binary can read the archive was already
		// decided, and it was decided against the migrations this binary
		// carries rather than against this file: an archive older than the
		// live database is the ordinary "restore last night's backup" case,
		// and refusing it -- which this code used to do -- was a bug.
		archiveVersion int64
		liveVersion    int64
		hadLive        bool
	}
	plan := make([]step, 0, len(result.Manifest.Databases))
	inArchive := map[string]bool{}
	for _, database := range result.Manifest.Databases {
		inArchive[database.Name] = true
		live, ok := locations[database.Name]
		if !ok || live == "" {
			return fmt.Errorf("归档含有 %s 库，而本部署没有为它配置存放位置", database.Name)
		}
		image := filepath.Join(result.ExtractedTo, filepath.FromSlash(database.Member))
		if _, err := os.Stat(image); err != nil {
			return fmt.Errorf("归档成员 %s 不在解包结果里：%w", database.Member, err)
		}
		entry := step{name: database.Name, live: live, image: image, archiveVersion: database.SchemaVersion}
		if _, err := os.Stat(live); err == nil {
			version, _, err := backup.SchemaVersionOf(live)
			if err != nil {
				return fmt.Errorf("读取当前 %s 库的 schema 版本：%w", database.Name, err)
			}
			entry.liveVersion = version
			entry.hadLive = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查当前 %s 库：%w", database.Name, err)
		}
		plan = append(plan, entry)
	}

	for _, entry := range plan {
		if entry.hadLive && entry.liveVersion != entry.archiveVersion {
			fmt.Printf("   %s：本机原为 schema %d，归档为 %d，下一次启动会把它前移到本二进制支持的版本\n",
				entry.name, entry.liveVersion, entry.archiveVersion)
		}
	}

	// A store the archive does not hold is left exactly as it is rather than
	// emptied: "the archive has no lease store" means the host it came from had
	// none, not that this host's should be deleted. Said out loud, because an
	// operator expecting a full replacement would otherwise not notice.
	for name, live := range locations {
		if inArchive[name] || live == "" {
			continue
		}
		if _, err := os.Stat(live); err == nil {
			fmt.Printf("注意：归档不含 %s 库，本机的 %s 保持原样\n", name, live)
		}
	}

	var replaced []string
	for _, entry := range plan {
		if err := replaceDatabaseFile(entry.image, entry.live); err != nil {
			return fmt.Errorf("替换 %s 库失败（已替换：%s；替换前副本：%s）：%w",
				entry.name, orNone(replaced), orAbsent(safety), err)
		}
		replaced = append(replaced, entry.name)
		fmt.Printf("已替换 %-8s ← %s\n", entry.name, entry.live)
	}

	fmt.Println("数据库已替换。归档中的配置文件刻意未应用——它描述的是归档来源主机，请自行比对后再决定是否采用。")
	fmt.Println("下一步：启动服务，让迁移把 schema 补齐到本二进制支持的版本。")
	return nil
}

// existingStores lists the stores this deployment has a file for.
//
// The mandate is dropped on purpose. snapshotTargets marks the control
// database as mandatory because a *backup* that silently omits it is the worst
// possible outcome of a command that reported success. A safety copy is the
// opposite situation: it records what is about to be replaced, and the honest
// answer on a host that has never run is "nothing".
func existingStores(cfg *config.Config) []backup.SnapshotTarget {
	var found []backup.SnapshotTarget
	for _, target := range snapshotTargets(cfg) {
		path := backup.DSNPath(target.DSN)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			// Not there, or not readable: either way there is nothing to
			// preserve, and the replace step reports the real problem if there
			// is one.
			continue
		}
		target.Optional = true
		found = append(found, target)
	}
	return found
}

// replaceDatabaseFile puts a restored database in place of a live one.
//
// The copy lands beside its destination and is renamed onto it, so the swap is
// atomic and cannot cross a filesystem boundary.
//
// The write-ahead log and shared-memory files next to the old database are
// removed first, because they belong to the file being replaced. This matters
// in one case: while another connection is still attached -- which is what an
// operator who ran --in-place without stopping the service leaves behind --
// SQLite cannot remove the log itself, and a log sitting beside a database it
// does not belong to is a corruption waiting for the next reader that adopts
// WAL mode, which the service does on startup. When nothing else is attached
// the log is already checkpointed away by the checks this function's caller
// ran, and the removal is a no-op.
func replaceDatabaseFile(image, live string) error {
	if err := os.MkdirAll(filepath.Dir(live), 0o755); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := os.Remove(live + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 %s%s：%w", live, suffix, err)
		}
	}
	temporary := live + ".restore-new"
	if err := copyRegularFile(image, temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, live); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}

func copyRegularFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func orNone(names []string) string {
	if len(names) == 0 {
		return "无"
	}
	return strings.Join(names, ", ")
}

// orAbsent names a path that may legitimately not exist, so a failure message
// does not send the operator looking for a file that was never written.
func orAbsent(path string) string {
	if strings.TrimSpace(path) == "" {
		return "无（替换前没有任何库）"
	}
	return path
}

// describeSnapshot prints what an archive holds. Every line comes from the
// manifest, so this is also the report of a verify-only read: what the operator
// sees is what the file claims, and the read has already checked every claim.
func describeSnapshot(manifest backup.SnapshotManifest, path string) {
	fmt.Printf("归档：%s\n", path)
	fmt.Printf("  格式版本 %d · 加密 %s · 创建于 %s\n",
		manifest.FormatVersion, manifest.Encryption, manifest.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	if manifest.Description != "" {
		fmt.Printf("  说明：%s\n", manifest.Description)
	}
	fmt.Println("  数据库：")
	for _, database := range manifest.Databases {
		fmt.Printf("    %-8s %10s  schema %-9s 表 %-4d ← %s\n",
			database.Name, humanBytes(database.SizeBytes), revisionLabel(database), database.Tables, database.Source)
	}
	for _, absent := range manifest.Absent {
		fmt.Printf("    %-8s %10s  %s ← %s\n", absent.Name, "不存在", reasonLabel(absent.Reason), absent.Source)
	}
	if len(manifest.ConfigFiles) > 0 {
		fmt.Println("  配置文件：")
		for _, file := range manifest.ConfigFiles {
			fmt.Printf("    %-22s %10s ← %s\n", file.Member, humanBytes(file.SizeBytes), file.Source)
		}
	}
	if len(manifest.Secrets.Inventory) > 0 {
		fmt.Println("  密钥盘点（只计数，不含值）：")
		for _, entry := range manifest.Secrets.Inventory {
			fmt.Printf("    %-24s %5d 条，其中已加密 %d 条\n",
				entry.Table+"."+entry.Column, entry.Rows, entry.Sealed)
		}
	}
	for _, label := range backup.SnapshotKeyLabels(manifest) {
		fmt.Printf("  密钥指纹 %s = %s\n", label, manifest.Secrets.Keys[label])
	}
	if manifest.Secrets.Note != "" {
		fmt.Printf("  注意：%s\n", manifest.Secrets.Note)
	}
}

// revisionLabel distinguishes a database that has never been migrated from one
// sitting at revision zero, which is the difference a restore cares about.
func revisionLabel(database backup.SnapshotDatabase) string {
	if !database.SchemaTablePresent {
		return "无迁移表"
	}
	return strconv.FormatInt(database.SchemaVersion, 10)
}

// reasonLabel renders the manifest's machine-readable reason for a human. The
// manifest keeps the English value: it is a field in a file that may be carried
// to a host where nothing else about this project is installed.
func reasonLabel(reason string) string {
	switch reason {
	case "no such file":
		return "文件不存在"
	default:
		return reason
	}
}

func humanBytes(size int64) string {
	switch {
	case size >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(size)/(1<<30))
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// --- DHCP high availability: the operator actions (ADR 0003) ---
