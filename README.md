# 语音转文字

<img width="588" height="278" alt="image" src="https://github.com/user-attachments/assets/bd1a17e9-539f-4188-aec7-62a39d33d9fb" />

## 音频转换

```
brew install ffmpeg
ffmpeg -i asr_example.wav -f s16le -acodec pcm_s16le -ar 16000 -ac 1 asr_example_16k.pcm
```
(macOS)

## 参考文档

> https://help.aliyun.com/zh/model-studio/real-time-speech-recognition?spm=a2c4g.11186623.help-menu-2400256.d_0_4_0.3dd73e98ljPRuA

> https://help.aliyun.com/zh/model-studio/websocket-for-paraformer-real-time-service?spm=a2c4g.11186623.help-menu-2400256.d_2_6_3_0_2.176c4462dbtNXv&scm=20140722.H_2856047._.OR_help-T_cn~zh-V_1#2942cede42z9e
