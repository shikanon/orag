# Demo Data

Use these remote test assets for multimodal ORAG validation. Do not download the long video in normal smoke tests; use it only for upload capability and large-file boundary checks.

| Name | Kind | URL | Usage |
| --- | --- | --- | --- |
| 测试图片1 | image | `https://lensrhyme.tos-cn-hongkong.volces.com/test/girl.jpg` | Image ingestion, metadata, and mixed-media prompt grounding. |
| 测试图片2 | image | `https://lensrhyme.tos-cn-hongkong.volces.com/test/man.jpg` | Image ingestion, person visual context, and image retrieval comparison. |
| 测试BGM音乐 | audio | `https://lensrhyme.tos-cn-hongkong.volces.com/test/music.mp4` | BGM/audio upload, media metadata extraction, and multimodal attachment handling. |
| 测试视频1 | video | `https://lensrhyme.tos-cn-hongkong.volces.com/test/TestVideo.mp4` | Short video upload, video metadata, and visual/audio multimodal smoke. |
| 测试视频2 | video | `https://lensrhyme.tos-cn-hongkong.volces.com/test/gamevideo.mp4` | Gameplay video upload and video retrieval comparison. |
| 测试长视频 | video | `https://lensrhyme.tos-cn-hongkong.volces.com/test/TestLongVideo.mp4` | Large-file upload and resumable upload boundary testing only. |
| 测试脚本 | document | `https://lensrhyme.tos-cn-hongkong.volces.com/test/test.docx` | Script document upload, docx parsing, and cross-modal script-to-media grounding. |

Notes:

- Treat `music.mp4` as the BGM/audio fixture.
- Treat `TestLongVideo.mp4` as `large_file_only=true`.
- Combine the docx script with image and video assets when testing script-to-media grounding.
