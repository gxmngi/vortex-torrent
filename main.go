package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gxmngi/vortex-torrent/client"
	"github.com/gxmngi/vortex-torrent/torrentfile"
)

const Version = "0.1.0"

func printBanner() {
	fmt.Println("==================================================")
	fmt.Printf(" VortexTorrent Engine v%s (Go Native)\n", Version)
	fmt.Println(" Concurrent BitTorrent P2P Downloader")
	fmt.Println("==================================================")
}

func printHelp() {
	printBanner()
	fmt.Println("Usage:")
	fmt.Println("  vortex-torrent info <file.torrent>")
	fmt.Println("      Inspect torrent metadata and 20-byte SHA-1 info_hash")
	fmt.Println("  vortex-torrent peers <file.torrent>")
	fmt.Println("      Query public tracker and list live peer endpoints in swarm")
	fmt.Println("  vortex-torrent handshake <file.torrent>")
	fmt.Println("      Perform 68-byte TCP handshake with a live peer")
	fmt.Println("  vortex-torrent download <file.torrent> <output-file>")
	fmt.Println("      Download torrent concurrently from swarm peers")
	fmt.Println("==================================================")
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "info":
		if len(os.Args) < 3 {
			log.Fatal("Usage: vortex-torrent info <file.torrent>")
		}
		path := os.Args[2]
		tf, err := torrentfile.Open(path)
		if err != nil {
			log.Fatalf("Error opening torrent file: %v", err)
		}

		printBanner()
		fmt.Printf("Tracker URL  : %s\n", tf.Announce)
		fmt.Printf("File Name    : %s\n", tf.Name)
		fmt.Printf("File Length  : %d bytes (%.2f MB)\n", tf.Length, float64(tf.Length)/(1024*1024))
		fmt.Printf("Piece Length : %d bytes\n", tf.PieceLength)
		fmt.Printf("Total Pieces : %d\n", len(tf.PieceHashes))
		fmt.Printf("Info Hash    : %x\n", tf.InfoHash)

	case "peers":
		if len(os.Args) < 3 {
			log.Fatal("Usage: vortex-torrent peers <file.torrent>")
		}
		path := os.Args[2]
		tf, err := torrentfile.Open(path)
		if err != nil {
			log.Fatalf("Error opening torrent file: %v", err)
		}

		peerID := torrentfile.GeneratePeerID()

		fmt.Printf("Querying tracker %s for swarm peers...\n", tf.Announce)
		peerList, err := tf.RequestPeers(peerID, 6881)
		if err != nil {
			log.Fatalf("Tracker query failed: %v", err)
		}

		fmt.Printf("Found %d active peers in swarm:\n", len(peerList))
		for i, p := range peerList {
			fmt.Printf("  [%02d] %s\n", i+1, p.String())
		}

	case "handshake":
		if len(os.Args) < 3 {
			log.Fatal("Usage: vortex-torrent handshake <file.torrent>")
		}
		path := os.Args[2]
		tf, err := torrentfile.Open(path)
		if err != nil {
			log.Fatalf("Error opening torrent file: %v", err)
		}

		peerID := torrentfile.GeneratePeerID()

		peerList, err := tf.RequestPeers(peerID, 6881)
		if err != nil || len(peerList) == 0 {
			log.Fatalf("No peers found: %v", err)
		}

		peer := peerList[0]
		printBanner()
		fmt.Printf("Attempting BitTorrent TCP handshake with peer %s...\n", peer.String())
		c, err := client.New(peer, peerID, tf.InfoHash)
		if err != nil {
			log.Fatalf("Handshake failed: %v", err)
		}
		defer func() { _ = c.Conn.Close() }()

		fmt.Printf("SUCCESS! 68-Byte Handshake established with %s!\n", peer.String())
		fmt.Printf("Peer initial choked state: %v\n", c.Choked)
		fmt.Printf("Peer bitfield length     : %d bytes\n", len(c.Bitfield))

	case "download":
		if len(os.Args) < 4 {
			log.Fatal("Usage: vortex-torrent download <file.torrent> <output-file>")
		}
		torrentPath := os.Args[2]
		outputPath := os.Args[3]

		printBanner()
		tf, err := torrentfile.Open(torrentPath)
		if err != nil {
			log.Fatalf("Error opening torrent file: %v", err)
		}

		err = tf.DownloadToFile(outputPath)
		if err != nil {
			log.Fatalf("Download failed: %v", err)
		}

		fmt.Printf("\nSUCCESS: Successfully downloaded and verified %s -> %s\n", tf.Name, outputPath)

	case "help", "--help", "-h":
		printHelp()

	default:
		printHelp()
		os.Exit(1)
	}
}
