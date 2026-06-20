// Mirror of plan.AudioFormat (plan/timeline.go).
// Phase-one Kokoro defaults: 24 kHz mono PCM/WAV.

export interface AudioFormat {
  sample_rate: number
  channels: number
  encoding: string
}
