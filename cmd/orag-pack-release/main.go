// orag-pack-release creates an immutable local Text-RAG release directory.
// Uploading is a separate explicit command so no build can publish by accident.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shikanon/orag/internal/packrelease"
)

func main() {
	var source, output, version, publishRoot, verifyRoot, publicBase string
	var quickMaxBytes int64
	flag.StringVar(&source, "source", "", "clean CRUD-RAG git checkout")
	flag.StringVar(&output, "output", "", "empty output directory")
	flag.StringVar(&version, "version", packrelease.DefaultVersion, "immutable pack version")
	flag.Int64Var(&quickMaxBytes, "quick-max-bytes", packrelease.DefaultQuickMaxBytes, "maximum quick corpus bytes")
	flag.StringVar(&publishRoot, "publish", "", "completed release root to publish (explicit)")
	flag.StringVar(&verifyRoot, "verify-public", "", "completed release root to verify anonymously")
	flag.StringVar(&publicBase, "public-base-url", packrelease.DefaultPublicBaseURL, "public tutorial-pack base URL")
	flag.Parse()
	if publishRoot != "" {
		err := packrelease.Publish(context.Background(), packrelease.PublishConfig{ReleaseRoot: publishRoot, Endpoint: os.Getenv("OBJECT_STORAGE_ENDPOINT"), Bucket: os.Getenv("OBJECT_STORAGE_BUCKET_NAME"), AccessKey: os.Getenv("OBJECT_STORAGE_ACCESS_KEY_ID"), SecretKey: os.Getenv("OBJECT_STORAGE_ACCESS_KEY_SECRET")})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("published %s\n", publishRoot)
		return
	}
	if verifyRoot != "" {
		if err := packrelease.VerifyPublic(context.Background(), verifyRoot, publicBase); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("verified %s\n", verifyRoot)
		return
	}
	if source == "" || output == "" {
		log.Fatal("-source and -output are required")
	}
	release, err := packrelease.Build(packrelease.BuildConfig{SourceDir: source, OutputDir: output, Version: version, QuickMaxBytes: quickMaxBytes})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("built %s from %s (quick=%d benchmark=%d)\n", release.Root, release.SourceCommit, release.QuickBytes, release.BenchmarkBytes)
}
