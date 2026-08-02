import AppKit
import Foundation
import Vision

struct Label: Codable {
    let name: String
    let confidence: Float
}

struct FrameResult: Codable {
    let labels: [Label]
    let recognized_text: [String]
    let error: String?
}

func cgImage(at path: String) -> CGImage? {
    guard let image = NSImage(contentsOfFile: path) else {
        return nil
    }
    var rect = NSRect(origin: .zero, size: image.size)
    return image.cgImage(forProposedRect: &rect, context: nil, hints: nil)
}

func analyze(path: String) -> FrameResult {
    guard let image = cgImage(at: path) else {
        return FrameResult(labels: [], recognized_text: [], error: "could not load image")
    }

    let classify = VNClassifyImageRequest()
    let recognizeText = VNRecognizeTextRequest()
    recognizeText.recognitionLevel = .accurate
    recognizeText.automaticallyDetectsLanguage = true

    do {
        try VNImageRequestHandler(cgImage: image).perform([classify, recognizeText])
        let labels = (classify.results ?? [])
            .filter { $0.confidence >= 0.03 }
            .prefix(16)
            .map { Label(name: $0.identifier, confidence: $0.confidence) }

        var seenText = Set<String>()
        let recognizedText = (recognizeText.results ?? [])
            .compactMap { $0.topCandidates(1).first?.string.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty && seenText.insert($0).inserted }
            .prefix(20)

        return FrameResult(labels: labels, recognized_text: Array(recognizedText), error: nil)
    } catch {
        return FrameResult(labels: [], recognized_text: [], error: error.localizedDescription)
    }
}

let results = CommandLine.arguments.dropFirst().map(analyze)
do {
    let data = try JSONEncoder().encode(results)
    FileHandle.standardOutput.write(data)
} catch {
    FileHandle.standardError.write(Data(error.localizedDescription.utf8))
    exit(1)
}
