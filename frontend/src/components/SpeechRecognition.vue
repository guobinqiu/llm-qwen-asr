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
      if (this.recording) return

      this.recording = true

      // 初始化 WebSocket（如果尚未连接）
      if (!this.ws || this.ws.readyState === WebSocket.CLOSED) {
        this.ws = new WebSocket(`ws://${location.hostname}:8080/ws`)
        this.ws.binaryType = 'arraybuffer'

        this.ws.onmessage = (e) => {
          if (!this.recording) return
          const text = e.data.trim()
          console.log('Received:', text)
          if (!text) return
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
      this.scriptNode = this.audioCtx.createScriptProcessor(8192, 1, 1)

      this.scriptNode.onaudioprocess = (e) => {
        if (!this.recording || this.ws.readyState !== WebSocket.OPEN) return
        const input = e.inputBuffer.getChannelData(0)
        const pcm = this.resampleToPCM(input, inputRate, this.sampleRate)
        if (pcm) {
          this.ws.send(pcm)
        }
      }

      source.connect(this.scriptNode)
      this.scriptNode.connect(this.audioCtx.destination)
    },

    stop() {
      if (!this.recording) return
      this.recording = false

      // 延迟500ms，确保音频数据完全发送
      setTimeout(() => {
        this.scriptNode && this.scriptNode.disconnect()
        this.audioCtx && this.audioCtx.close()
        this.audioCtx = null
      }, 500)
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
  }
}
</script>

<style>
button.recording {
  background-color: red;
  color: white;
}
</style>
