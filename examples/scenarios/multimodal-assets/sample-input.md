# Sample Input

Test request: validate ORAG multimodal upload and retrieval preparation with the approved remote asset set.

Asset categories:

- Images: `girl.jpg`, `man.jpg`
- BGM/audio: `music.mp4`
- Short videos: `TestVideo.mp4`, `gamevideo.mp4`
- Long video: `TestLongVideo.mp4`, used only for upload-boundary testing
- Script document: `test.docx`

Goal: confirm the manifest covers every required media type while avoiding normal smoke-test downloads of the long video.
