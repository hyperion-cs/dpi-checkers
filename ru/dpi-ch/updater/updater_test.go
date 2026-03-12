package updater

import (
	"context"
	"path"
	"testing"
	"time"
)

func Test1(t *testing.T) {
	owner, repo, fn, branch := "Loyalsoldier", "geoip", "GeoLite2-ASN-Blocks-IPv4.csv", "release"
	dir := "../data/updater_testdata/"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	remoteHash, err := remoteHash(ctx, owner, repo, fn, branch)
	if err != nil {
		t.Fatalf("RemoteHash: %e", err)
	}

	hashDst := path.Join(dir, fn)
	if err = writeLocalHash(hashDst, remoteHash); err != nil {
		t.Fatalf("WriteLocalHash: %e", err)
	}

	localHash, err := readLocalHash(hashDst)
	if err != nil {
		t.Fatalf("ReadLocalHash: %e", err)
	}
	if localHash != remoteHash {
		t.Fatalf("ReadLocalHash: got %v, want %v", localHash, remoteHash)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = download(ctx, contentUrl(owner, repo, fn, branch), path.Join(dir, fn))
	if err != nil {
		t.Fatalf("Download: %e", err)
	}
}
