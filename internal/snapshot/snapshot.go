package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/alicanalbayrak/sikifanso/internal/paths"
	"github.com/alicanalbayrak/sikifanso/internal/session"
)

// Meta holds snapshot metadata.
type Meta struct {
	Name        string    `json:"name"`
	ClusterName string    `json:"clusterName"`
	CreatedAt   time.Time `json:"createdAt"`
	CLIVersion  string    `json:"cliVersion"`
}

const (
	snapshotsDir = "snapshots"
	metaFile     = "snapshot-meta.yaml"
	sessionFile  = "session.yaml"
	gitopsPrefix = "gitops"
)


// SnapshotsDir returns the snapshots directory, creating it if it doesn't exist.
func SnapshotsDir() (string, error) {
	root, err := paths.RootDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, snapshotsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating snapshots directory: %w", err)
	}
	return dir, nil
}

// Capture creates a tar.gz snapshot of the cluster's session and gitops tree.
// It returns the path to the created archive.
func Capture(clusterName, snapshotName, cliVersion string) (archivePath string, retErr error) {
	sess, err := session.Load(clusterName)
	if err != nil {
		return "", fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	sessDir, err := session.Dir(clusterName)
	if err != nil {
		return "", fmt.Errorf("getting session directory: %w", err)
	}

	dir, err := SnapshotsDir()
	if err != nil {
		return "", err
	}

	archivePath = filepath.Join(dir, snapshotName+".tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("creating archive file: %w", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("closing archive file: %w", cErr)
		}
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		if cErr := gw.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("closing gzip writer: %w", cErr)
		}
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		if cErr := tw.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("closing tar writer: %w", cErr)
		}
	}()

	// Write snapshot-meta.yaml
	meta := Meta{
		Name:        snapshotName,
		ClusterName: clusterName,
		CreatedAt:   time.Now(),
		CLIVersion:  cliVersion,
	}
	metaData, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshaling snapshot metadata: %w", err)
	}
	if err := writeTarBytes(tw, metaFile, metaData); err != nil {
		return "", fmt.Errorf("writing metadata to archive: %w", err)
	}

	// Write session.yaml
	sessionPath := filepath.Join(sessDir, sessionFile)
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		return "", fmt.Errorf("reading session file: %w", err)
	}
	if err := writeTarBytes(tw, sessionFile, sessionData); err != nil {
		return "", fmt.Errorf("writing session to archive: %w", err)
	}

	// Walk and archive the entire gitops directory (including .git/)
	gitopsPath := sess.GitOpsPath
	if err := archiveDir(tw, gitopsPath, gitopsPrefix); err != nil {
		return "", fmt.Errorf("archiving gitops directory: %w", err)
	}

	return archivePath, nil
}

// List returns metadata for all snapshots, sorted by CreatedAt descending.
// If the snapshots directory doesn't exist, it returns nil, nil.
func List() ([]Meta, error) {
	root, err := paths.RootDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(root, snapshotsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading snapshots directory: %w", err)
	}

	var metas []Meta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}

		archivePath := filepath.Join(dir, e.Name())
		m, err := readMeta(archivePath)
		if err != nil {
			continue // skip archives without valid metadata
		}
		metas = append(metas, *m)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].CreatedAt.After(metas[j].CreatedAt)
	})

	return metas, nil
}

// Restore extracts a snapshot archive and restores the session and gitops tree.
// It returns the restored session and the gitops path.
func Restore(snapshotName string) (*session.Session, string, error) {
	dir, err := SnapshotsDir()
	if err != nil {
		return nil, "", err
	}

	archivePath := filepath.Join(dir, snapshotName+".tar.gz")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("snapshot %q not found", snapshotName)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, "", fmt.Errorf("opening snapshot archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	// First pass: extract session.yaml to determine the cluster name.
	tr := tar.NewReader(gr)
	var sess session.Session
	var foundSession bool

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Name == sessionFile {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, "", fmt.Errorf("reading session from archive: %w", err)
			}
			if err := yaml.Unmarshal(data, &sess); err != nil {
				return nil, "", fmt.Errorf("parsing session from archive: %w", err)
			}
			foundSession = true
			break
		}
	}
	if !foundSession {
		return nil, "", fmt.Errorf("session.yaml not found in snapshot %q", snapshotName)
	}

	sessDir, err := session.Dir(sess.ClusterName)
	if err != nil {
		return nil, "", fmt.Errorf("getting session directory: %w", err)
	}
	gitopsTarget := filepath.Join(sessDir, gitopsPrefix)

	// Second pass: extract gitops/ contents.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("seeking archive: %w", err)
	}
	gr2, err := gzip.NewReader(f)
	if err != nil {
		return nil, "", fmt.Errorf("creating gzip reader for second pass: %w", err)
	}
	defer func() { _ = gr2.Close() }()

	tr2 := tar.NewReader(gr2)
	if err := extractGitops(tr2, gitopsTarget); err != nil {
		return nil, "", fmt.Errorf("extracting gitops: %w", err)
	}

	// Update session paths and save.
	sess.GitOpsPath = gitopsTarget
	if err := session.Save(&sess); err != nil {
		return nil, "", fmt.Errorf("saving restored session: %w", err)
	}

	return &sess, gitopsTarget, nil
}

// Delete removes a snapshot archive. Returns an error if the file doesn't exist.
func Delete(snapshotName string) error {
	dir, err := SnapshotsDir()
	if err != nil {
		return err
	}

	archivePath := filepath.Join(dir, snapshotName+".tar.gz")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %q not found", snapshotName)
	}

	if err := os.Remove(archivePath); err != nil {
		return fmt.Errorf("removing snapshot %q: %w", snapshotName, err)
	}
	return nil
}

// writeTarBytes writes a single file entry to the tar archive.
func writeTarBytes(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// archiveDir walks baseDir recursively and adds all regular files and
// directories to the tar writer. Symlinks are skipped. Paths in the archive
// are prefixed with the given prefix.
func archiveDir(tw *tar.Writer, baseDir, prefix string) error {
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		archiveName := filepath.ToSlash(filepath.Join(prefix, rel))

		if info.IsDir() {
			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     archiveName + "/",
				Mode:     int64(info.Mode().Perm()),
				ModTime:  info.ModTime(),
			}
			return tw.WriteHeader(hdr)
		}

		// Only add regular files.
		if !info.Mode().IsRegular() {
			return nil
		}

		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     archiveName,
			Mode:     int64(info.Mode().Perm()),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening file %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()

		_, err = io.Copy(tw, f)
		return err
	})
}

// readMeta opens a tar.gz archive and extracts the snapshot-meta.yaml entry.
func readMeta(archivePath string) (*Meta, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("metadata not found in %s", filepath.Base(archivePath))
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == metaFile {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			var m Meta
			if err := yaml.Unmarshal(data, &m); err != nil {
				return nil, err
			}
			return &m, nil
		}
	}
}

// extractGitops reads the tar stream and extracts entries under "gitops/" into
// the target directory. It validates paths to prevent directory traversal.
func extractGitops(tr *tar.Reader, targetDir string) error {
	cleanTarget := filepath.Clean(targetDir)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Only process entries under the gitops prefix.
		if !strings.HasPrefix(hdr.Name, gitopsPrefix+"/") && hdr.Name != gitopsPrefix {
			continue
		}

		// Strip the "gitops/" prefix to get relative path within the target.
		rel := strings.TrimPrefix(hdr.Name, gitopsPrefix+"/")
		if rel == "" {
			// This is the gitops root directory entry itself.
			if err := os.MkdirAll(cleanTarget, 0755); err != nil {
				return fmt.Errorf("creating target directory: %w", err)
			}
			continue
		}

		dest := filepath.Join(cleanTarget, filepath.FromSlash(rel))
		dest = filepath.Clean(dest)

		// Path traversal prevention: ensure dest stays within targetDir.
		if !strings.HasPrefix(dest, cleanTarget+string(os.PathSeparator)) && dest != cleanTarget {
			return fmt.Errorf("illegal path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, os.FileMode(hdr.Mode).Perm()); err != nil {
				return fmt.Errorf("creating directory %s: %w", rel, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", rel, err)
			}
			if err := extractFile(dest, hdr, tr); err != nil {
				return err
			}
		default:
			// Skip symlinks and other special types.
			continue
		}
	}
}

// extractFile writes a single regular file from the tar reader to disk.
func extractFile(dest string, hdr *tar.Header, tr *tar.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode).Perm())
	if err != nil {
		return fmt.Errorf("creating file %s: %w", dest, err)
	}

	if _, err := io.Copy(out, tr); err != nil {
		_ = out.Close()
		return fmt.Errorf("writing file %s: %w", dest, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("closing file %s: %w", dest, err)
	}
	return nil
}
