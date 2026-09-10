@preconcurrency import AVFoundation

struct DictationAudio {
    let buffer: AVAudioPCMBuffer
    let level: Float
}

nonisolated func dictationAudioTap(
    converter: AVAudioConverter,
    consume: @escaping @Sendable (Result<DictationAudio, Error>) -> Void
) -> AVAudioNodeTapBlock {
    let format = converter.outputFormat
    let source = converter.inputFormat
    return { buffer, _ in
        let capacity = AVAudioFrameCount(ceil(Double(buffer.frameLength) * format.sampleRate / source.sampleRate)) + 1
        guard let converted = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: capacity) else {
            consume(.failure(NSError(domain: NSOSStatusErrorDomain, code: Int(kAudio_MemFullError))))
            return
        }
        var supplied = false
        var error: NSError?
        converter.convert(to: converted, error: &error) { _, status in
            if supplied {
                status.pointee = .noDataNow
                return nil
            }
            supplied = true
            status.pointee = .haveData
            return buffer
        }
        if let error {
            consume(.failure(error))
            return
        }
        guard converted.frameLength > 0 else {
            return
        }
        var level: Float = 0
        if let samples = buffer.floatChannelData?[0], buffer.frameLength > 0 {
            var energy: Float = 0
            for index in 0..<Int(buffer.frameLength) {
                energy += samples[index] * samples[index]
            }
            level = min(1, sqrt(energy / Float(buffer.frameLength)))
        }
        consume(.success(DictationAudio(buffer: converted, level: level)))
    }
}
