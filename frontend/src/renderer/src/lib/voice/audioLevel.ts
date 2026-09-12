export function audioLevel(analyser: AnalyserNode, samples: Uint8Array<ArrayBuffer>): number {
  analyser.getByteTimeDomainData(samples)
  let energy = 0
  for (const sample of samples) {
    energy += ((sample - 128) / 128) ** 2
  }
  return Math.min(1, Math.sqrt(energy / samples.length) * 5)
}
