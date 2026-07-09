package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
)

type Asset struct {
	Name          string
	Kind          string
	URL           string
	Usage         string
	UploadTest    bool
	LargeFileOnly bool
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out io.Writer) error {
	assets := []Asset{
		{
			Name:       "测试图片1",
			Kind:       "image",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/girl.jpg",
			Usage:      "image ingestion, image metadata, and mixed-media prompt grounding",
			UploadTest: true,
		},
		{
			Name:       "测试图片2",
			Kind:       "image",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/man.jpg",
			Usage:      "image ingestion, face/person visual context, and image retrieval comparison",
			UploadTest: true,
		},
		{
			Name:       "测试BGM音乐",
			Kind:       "audio",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/music.mp4",
			Usage:      "audio or BGM upload, media metadata extraction, and multimodal attachment handling",
			UploadTest: true,
		},
		{
			Name:       "测试视频1",
			Kind:       "video",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/TestVideo.mp4",
			Usage:      "short video upload, video metadata, and visual/audio multimodal smoke",
			UploadTest: true,
		},
		{
			Name:       "测试视频2",
			Kind:       "video",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/gamevideo.mp4",
			Usage:      "gameplay video upload and video retrieval comparison",
			UploadTest: true,
		},
		{
			Name:          "测试长视频",
			Kind:          "video",
			URL:           "https://lensrhyme.tos-cn-hongkong.volces.com/test/TestLongVideo.mp4",
			Usage:         "large file upload and resumable upload boundary testing only",
			UploadTest:    true,
			LargeFileOnly: true,
		},
		{
			Name:       "测试脚本",
			Kind:       "document",
			URL:        "https://lensrhyme.tos-cn-hongkong.volces.com/test/test.docx",
			Usage:      "script document upload, docx parsing, and cross-modal script-to-media grounding",
			UploadTest: true,
		},
	}

	fmt.Fprintln(out, "scenario=multimodal-assets")
	fmt.Fprintln(out, "purpose=remote multimodal test asset manifest")
	fmt.Fprintf(out, "asset_count=%d\n", len(assets))
	for i, asset := range assets {
		parsed, err := url.Parse(asset.URL)
		if err != nil {
			return fmt.Errorf("parse %s: %w", asset.Name, err)
		}
		if parsed.Scheme != "https" {
			return fmt.Errorf("%s must use https: %s", asset.Name, asset.URL)
		}
		filename := path.Base(parsed.Path)
		if filename == "." || filename == "/" || filename == "" {
			return fmt.Errorf("%s missing filename in URL: %s", asset.Name, asset.URL)
		}
		extension := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
		fmt.Fprintf(out, "asset[%d].name=%s\n", i+1, asset.Name)
		fmt.Fprintf(out, "asset[%d].kind=%s\n", i+1, asset.Kind)
		fmt.Fprintf(out, "asset[%d].filename=%s\n", i+1, filename)
		fmt.Fprintf(out, "asset[%d].extension=%s\n", i+1, extension)
		fmt.Fprintf(out, "asset[%d].url=%s\n", i+1, asset.URL)
		fmt.Fprintf(out, "asset[%d].upload_test=%t\n", i+1, asset.UploadTest)
		fmt.Fprintf(out, "asset[%d].large_file_only=%t\n", i+1, asset.LargeFileOnly)
		fmt.Fprintf(out, "asset[%d].usage=%s\n", i+1, asset.Usage)
	}
	fmt.Fprintln(out, "usage_dimensions:")
	fmt.Fprintln(out, "- 使用方: 多模态上传、解析、检索、RAG 质量验证人员")
	fmt.Fprintln(out, "- 业务问题: 用统一远程素材覆盖图片、音频、视频、长视频、docx 脚本的测试路径")
	fmt.Fprintln(out, "- 输入数据: 2 张图片、1 个 BGM、2 个短视频、1 个长视频、1 个 docx 脚本")
	fmt.Fprintln(out, "- ORAG能力: 远程资产登记、上传链路、文档解析、媒体元数据、跨模态检索准备")
	fmt.Fprintln(out, "- 成功标准: 所有 URL 为 https、类型和用途明确，长视频只进入上传能力测试")
	return nil
}
