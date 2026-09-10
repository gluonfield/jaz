import AVFoundation
import Foundation
import Speech

let outputLock = NSLock()

func emit(_ value: [String: Any]) {
    outputLock.lock()
    defer { outputLock.unlock() }
    let data = try! JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
    FileHandle.standardOutput.write(data + Data([10]))
}

struct DictationError: LocalizedError {
    let errorDescription: String?

    init(_ message: String) {
        errorDescription = message
    }
}

@available(macOS 26.0, *)
@MainActor
final class Dictation {
    let engine = AVAudioEngine()
    var input: AsyncStream<AnalyzerInput>.Continuation?
    var analyzer: SpeechAnalyzer?
    var recording = false

    func run(locale: Locale, file: String?) async throws {
        guard SpeechTranscriber.isAvailable else {
            throw DictationError("On-device transcription is unavailable on this Mac.")
        }
        guard let supported = await SpeechTranscriber.supportedLocale(equivalentTo: locale) else {
            throw DictationError("On-device transcription does not support \(locale.localizedString(forIdentifier: locale.identifier) ?? locale.identifier).")
        }
        if file == nil {
            let allowed = await AVCaptureDevice.requestAccess(for: .audio)
            guard allowed else {
                throw DictationError("Allow microphone access for Jaz in System Settings → Privacy & Security → Microphone.")
            }
        }
        let transcriber = SpeechTranscriber(locale: supported, preset: .progressiveTranscription)
        if let installation = try await AssetInventory.assetInstallationRequest(supporting: [transcriber]) {
            emit(["type": "status", "phase": "downloading"])
            try await installation.downloadAndInstall()
        }
        let analyzer = SpeechAnalyzer(modules: [transcriber])
        self.analyzer = analyzer
        let results = Task {
            var finalized = ""
            for try await result in transcriber.results {
                let text = String(result.text.characters)
                if result.isFinal {
                    finalized += text
                }
                emit(["type": "result", "text": result.isFinal ? finalized : finalized + text])
            }
            return finalized
        }
        defer {
            results.cancel()
            stopCapture()
        }
        do {
            if let file {
                emit(["type": "status", "phase": "transcribing"])
                let audio = try AVAudioFile(forReading: URL(fileURLWithPath: file))
                try await analyzer.start(inputAudioFile: audio, finishAfterFile: true)
            } else {
                try await record(using: analyzer, transcriber: transcriber)
            }
            let text = try await results.value
            emit(["type": "complete", "text": text])
        } catch {
            await analyzer.cancelAndFinishNow()
            throw error
        }
    }

    func record(using analyzer: SpeechAnalyzer, transcriber: SpeechTranscriber) async throws {
        let node = engine.inputNode
        let source = node.outputFormat(forBus: 0)
        guard source.sampleRate > 0, source.channelCount > 0,
              let format = await SpeechAnalyzer.bestAvailableAudioFormat(compatibleWith: [transcriber]),
              let converter = AVAudioConverter(from: source, to: format) else {
            throw DictationError("No usable microphone was found.")
        }
        let (stream, continuation) = AsyncStream<AnalyzerInput>.makeStream()
        input = continuation
        try await analyzer.prepareToAnalyze(in: format)
        try await analyzer.start(inputSequence: stream)
        let tap = dictationAudioTap(converter: converter) { result in
            switch result {
            case .success(let audio):
                continuation.yield(AnalyzerInput(buffer: audio.buffer))
                emit(["type": "level", "level": audio.level])
            case .failure(let error):
                emit(["type": "error", "message": error.localizedDescription])
                continuation.finish()
            }
        }
        node.installTap(onBus: 0, bufferSize: 4096, format: source, block: tap)
        recording = true
        engine.prepare()
        try engine.start()
        emit(["type": "status", "phase": "recording"])
    }

    func stopCapture() {
        if recording {
            engine.stop()
            engine.inputNode.removeTap(onBus: 0)
            recording = false
        }
        input?.finish()
        input = nil
    }

    func stop() async throws {
        guard recording, let analyzer else {
            return
        }
        stopCapture()
        emit(["type": "status", "phase": "transcribing"])
        try await analyzer.finalizeAndFinishThroughEndOfInput()
    }
}

@main
struct Main {
    @MainActor
    static func main() async {
        let arguments = CommandLine.arguments
        guard #available(macOS 26.0, *) else {
            emit(["available": false, "reason": "Native dictation requires macOS 26 or later."])
            return
        }
        let localeIndex = arguments.firstIndex(of: "--locale")
        let locale = localeIndex.flatMap { arguments.indices.contains($0 + 1) ? Locale(identifier: arguments[$0 + 1]) : nil } ?? Locale.current
        if arguments.contains("--check") {
            let supported = await SpeechTranscriber.supportedLocale(equivalentTo: locale)
            let available = SpeechTranscriber.isAvailable && supported != nil
            emit(available ? ["available": true] : ["available": false, "reason": "On-device dictation is unavailable for this Mac or language."])
            return
        }
        let fileIndex = arguments.firstIndex(of: "--file")
        let file = fileIndex.flatMap { arguments.indices.contains($0 + 1) ? arguments[$0 + 1] : nil }
        let dictation = Dictation()
        if file == nil {
            DispatchQueue.global().async {
                while let command = readLine() {
                    if command == "stop" {
                        Task { @MainActor in
                            do {
                                try await dictation.stop()
                            } catch {
                                emit(["type": "error", "message": error.localizedDescription])
                                exit(1)
                            }
                        }
                    }
                }
                exit(0)
            }
        }
        do {
            try await dictation.run(locale: locale, file: file)
        } catch {
            emit(["type": "error", "message": error.localizedDescription])
            exit(1)
        }
    }
}
