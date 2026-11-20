package torrent

import "errors"

// SetSpeedLimits changes the shared torrent session payload budgets in KiB/s.
// Zero is unlimited. Existing peers and webseeds observe the new setting.
func (s *Session) SetSpeedLimits(downloadKiB, uploadKiB int64) error {
	if downloadKiB < 0 || downloadKiB > 1<<20 || uploadKiB < 0 || uploadKiB > 1<<20 {
		return errors.New("torrent speed limit outside supported range")
	}
	s.bucketDownload.Set(downloadKiB * 1024)
	s.bucketUpload.Set(uploadKiB * 1024)
	return nil
}
