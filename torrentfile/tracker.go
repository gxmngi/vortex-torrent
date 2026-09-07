package torrentfile

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gxmngi/vortex-torrent/bencode"
	"github.com/gxmngi/vortex-torrent/peers"
)

// BuildTrackerURL constructs the GET query URL for announcing to the BitTorrent tracker.
func (t *TorrentFile) BuildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(t.Length)},
	}
	base.RawQuery = params.Encode()
	return base.String(), nil
}

// RequestPeers connects to the tracker and retrieves the list of active peers in the swarm.
func (t *TorrentFile) RequestPeers(peerID [20]byte, port uint16) ([]peers.Peer, error) {
	trackerURL, err := t.BuildTrackerURL(peerID, port)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(trackerURL)
	if err != nil {
		return nil, fmt.Errorf("tracker request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker returned HTTP status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading tracker response body: %w", err)
	}

	decoded, err := bencode.Unmarshal(body)
	if err != nil {
		return nil, fmt.Errorf("decoding tracker response: %w", err)
	}

	dict, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tracker response structure")
	}

	if failureReason, ok := dict["failure reason"].(string); ok && failureReason != "" {
		return nil, fmt.Errorf("tracker failure reason: %s", failureReason)
	}

	peersBinStr, ok := dict["peers"].(string)
	if !ok {
		return nil, fmt.Errorf("tracker response missing 'peers' field")
	}

	return peers.Unmarshal([]byte(peersBinStr))
}
