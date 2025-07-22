<template>
  <div>
    <h2>阿里云语音识别测试</h2>

    <button
      @mousedown="start"
      @mouseup="stop"
      @touchstart.prevent="start"
      @touchend.prevent="stop"
      :class="{ recording: recording }"
    >
      {{ recording ? '🎙️ 识别中...（松开停止）' : '按住说话' }}
    </button>

    <div>
      <h3>识别结果：</h3>
      <div>{{ currentTranscript }}</div>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      ws: null,
      audioCtx: null,
      scriptNode: null,
      recording: false,
      currentTranscript: '',
      sampleRate: 16000,
      audioStream: null,
    }
  },
  methods: {
    async start() {
      this.recording = true

      // 初始化 WebSocket（如果尚未连接）
      if (!this.ws || this.ws.readyState === WebSocket.CLOSED) {
        this.ws = new WebSocket(`ws://${location.hostname}:8080/ws`)
        this.ws.binaryType = 'arraybuffer'

        this.ws.onmessage = (e) => {
          const text = e.data.trim()
          console.log('Received:', text)
          this.currentTranscript = text
        }

        this.ws.onerror = () => console.error('WebSocket错误')
        this.ws.onclose = () => console.log('WebSocket已关闭')
      }

      if (!this.audioStream) {
        this.audioStream = await navigator.mediaDevices.getUserMedia({ audio: true })
      }

      this.audioCtx = new (window.AudioContext || window.webkitAudioContext)()
      const inputRate = this.audioCtx.sampleRate
      const source = this.audioCtx.createMediaStreamSource(this.audioStream)
      this.scriptNode = this.audioCtx.createScriptProcessor(1024, 1, 1)

      this.scriptNode.onaudioprocess = (e) => {
        const input = e.inputBuffer.getChannelData(0)
        const pcm = this.resampleToPCM(input, inputRate, this.sampleRate)
        if (pcm) {
          this.ws.send(pcm)
        }
      }

      source.connect(this.scriptNode)
      this.scriptNode.connect(this.audioCtx.destination)

      // 启动定时器，每隔 5 秒发送一次静音音频
      // this.silenceInterval = setInterval(() => {
      //   if (this.ws.readyState === WebSocket.OPEN) {
      //     this.sendSilentAudio()
      //   }
      // }, 5000)
    },

    stop() {
      this.recording = false

      // 清理定时器，停止发送静音音频
      // if (this.silenceInterval) {
      //   clearInterval(this.silenceInterval)
      //   this.silenceInterval = null
      // }

      // 延迟500ms，确保音频数据完全发送
      // setTimeout(() => {
      //   this.scriptNode && this.scriptNode.disconnect()
      //   this.audioCtx && this.audioCtx.close()
      //   this.audioCtx = null
      // }, 500)
    },

    resampleToPCM(input, fromRate, toRate) {
      const ratio = fromRate / toRate
      const newLen = Math.round(input.length / ratio)
      const output = new Int16Array(newLen)
      for (let i = 0; i < newLen; i++) {
        const s = Math.max(-1, Math.min(1, input[Math.floor(i * ratio)]))
        output[i] = s < 0 ? s * 0x8000 : s * 0x7FFF
      }
      return new Uint8Array(output.buffer)
    },
     // 发送静音音频
    // sendSilentAudio() {
    //   const silentAudio = new ArrayBuffer(1024);  // 创建一个空的音频数据块，表示静音（例如全零的 PCM 数据）
    //   if (this.ws.readyState === WebSocket.OPEN) {
    //     this.ws.send(silentAudio)
    //   }
    // },
  }
}
</script>

<style>
button.recording {
  background-color: red;
  color: white;
}
</style>
