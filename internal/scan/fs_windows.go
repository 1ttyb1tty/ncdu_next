//go:build windows

package scan

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// winVolume is one enumerated volume and the mount points that reach it.
type winVolume struct {
	root        string
	mountPoints []string
}

// osMountTable enumerates Windows volumes in two phases, so a scan never blocks
// on an unrelated, possibly disconnected, network mapping:
//
//  1. FindFirstVolume/FindNextVolume plus GetVolumePathNamesForVolumeName give
//     every volume the OS enumerates and the drive letters / mounted folders
//     that reach it, from local metadata only. Mapped network drives have no
//     volume GUID path and so are not enumerated here.
//  2. GetVolumeInformationW is asked for the filesystem name and serial only for
//     those local volumes.
//
// A path not covered by the resulting table (for example a UNC root) resolves to
// no entry, and the caller reports the type as unavailable with a reason.
func osMountTable() ([]FSInfo, error) {
	vols, err := listVolumes()
	if err != nil {
		return nil, err
	}
	var out []FSInfo
	for _, v := range vols {
		serial, fsname := volumeInfo(v.root)
		avail, reason := volumeAvailability(fsname)
		for _, mp := range v.mountPoints {
			out = append(out, FSInfo{
				MountPoint: mp,
				FSType:     fsname,
				Avail:      avail,
				Device:     serial,
				Reason:     reason,
			})
		}
	}
	return out, nil
}

// volumeAvailability decides a volume's state from its filesystem name. An empty
// name means GetVolumeInformation could not read the volume (for example an
// offline, spun-down, or disconnected one), which is "unavailable" — not
// "available" with a blank type. Treating it as available would misreport the
// state and suppress the --one-file-system fail-open warning.
func volumeAvailability(fsname string) (FSAvail, string) {
	if fsname == "" {
		return FSUnavailable, "volume information read failed (offline, disconnected, or unreadable volume)"
	}
	return FSAvailable, ""
}

// listVolumes enumerates every volume and the mount points reaching it.
func listVolumes() ([]winVolume, error) {
	buf := make([]uint16, windows.MAX_PATH+1)
	h, err := windows.FindFirstVolume(&buf[0], uint32(len(buf)))
	if err != nil {
		return nil, err
	}
	defer windows.FindVolumeClose(h) // nolint:errcheck
	var out []winVolume
	for {
		root := windows.UTF16ToString(buf)
		if mps := volumeMountPoints(root); len(mps) > 0 {
			out = append(out, winVolume{root: root, mountPoints: mps})
		}
		if err := windows.FindNextVolume(h, &buf[0], uint32(len(buf))); err != nil {
			break
		}
	}
	return out, nil
}

// volumeMountPoints returns the drive letters and mounted folders that reach the
// volume with the given GUID path.
func volumeMountPoints(root string) []string {
	rootp, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return nil
	}
	// First call with a zero-length buffer only to learn the required length.
	var needed uint32
	_ = windows.GetVolumePathNamesForVolumeName(rootp, nil, 0, &needed) // nolint:errcheck
	if needed == 0 {
		return nil
	}
	buf := make([]uint16, needed)
	if err := windows.GetVolumePathNamesForVolumeName(rootp, &buf[0], uint32(len(buf)), &needed); err != nil {
		return nil
	}
	return splitMultiSZ(buf)
}

// splitMultiSZ splits a double-NUL-terminated UTF-16 multi-string.
func splitMultiSZ(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i == start {
			break
		}
		out = append(out, windows.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	return out
}

// volumeInfo returns the volume serial number and filesystem name, or zero
// values when the OS cannot answer (for example a disconnected volume).
func volumeInfo(root string) (uint64, string) {
	rootp, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, ""
	}
	var serial uint32
	fsBuf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(rootp, nil, 0, &serial, nil, nil, &fsBuf[0], uint32(len(fsBuf))); err != nil {
		return 0, ""
	}
	return uint64(serial), windows.UTF16ToString(fsBuf)
}

// pathPrefix reports whether path is at or under mountPoint, comparing at a
// separator boundary. Windows paths are case-insensitive.
func pathPrefix(mountPoint, path string) bool {
	mp := strings.ToLower(filepath.Clean(mountPoint))
	p := strings.ToLower(filepath.Clean(path))
	if mp == p {
		return true
	}
	sep := string(filepath.Separator)
	if strings.HasSuffix(mp, sep) {
		return strings.HasPrefix(p, mp)
	}
	return strings.HasPrefix(p, mp+sep)
}
