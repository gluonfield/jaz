@preconcurrency import AVFoundation

@main
struct AudioTest {
    @MainActor
    static func main() async throws {
        for rate in [44100.0, 48000.0] {
            let source = AVAudioFormat(standardFormatWithSampleRate: rate, channels: 2)!
            let target = AVAudioFormat(standardFormatWithSampleRate: 16000, channels: 1)!
            let buffer = AVAudioPCMBuffer(pcmFormat: source, frameCapacity: AVAudioFrameCount(rate / 10))!
            buffer.frameLength = buffer.frameCapacity
            for channel in 0..<2 {
                for index in 0..<Int(buffer.frameLength) {
                    buffer.floatChannelData![channel][index] = 0.25
                }
            }
            let converter = AVAudioConverter(from: source, to: target)!
            let audio = try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<DictationAudio, Error>) in
                let tap = dictationAudioTap(converter: converter) { result in
                    dispatchPrecondition(condition: .notOnQueue(.main))
                    continuation.resume(with: result)
                }
                DispatchQueue.global().async {
                    tap(buffer, AVAudioTime(sampleTime: 0, atRate: rate))
                }
            }
            precondition(audio.buffer.format == target)
            precondition(audio.buffer.frameLength > 0 && audio.buffer.frameLength <= 1600)
            precondition(abs(audio.level - 0.25) < 0.001)
            let last = Int(audio.buffer.frameLength) - 1
            precondition(abs(audio.buffer.floatChannelData![0][last] - 0.25) < 0.001)
        }
        let format = AVAudioFormat(standardFormatWithSampleRate: 16000, channels: 1)!
        let silence = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: 1600)!
        silence.frameLength = 1600
        silence.floatChannelData![0].initialize(repeating: 0, count: 1600)
        let audio = try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<DictationAudio, Error>) in
            let tap = dictationAudioTap(converter: AVAudioConverter(from: format, to: format)!) {
                continuation.resume(with: $0)
            }
            DispatchQueue.global().async {
                tap(silence, AVAudioTime(sampleTime: 0, atRate: 16000))
            }
        }
        precondition(audio.level == 0)
        precondition(audio.buffer.frameLength == 1600)
        print("PASS: background audio callback, 44.1/48 kHz stereo conversion, mono samples, levels, and silence.")
    }
}
