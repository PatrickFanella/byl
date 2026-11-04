import SwiftUI
import AVKit
import InvisibleMuseumKit

/// Horizontal gallery of a moment's media with caption, credit, and licence under each item.
struct MediaGalleryView: View {
    let media: [MediaAsset]

    var body: some View {
        ScrollView(.horizontal) {
            HStack(alignment: .top, spacing: Museum.Spacing.md) {
                ForEach(media) { asset in
                    MediaTile(asset: asset)
                        .frame(width: 300)
                }
            }
        }
        .scrollIndicators(.hidden)
        .scrollTargetBehavior(.viewAligned)
    }
}

struct MediaTile: View {
    let asset: MediaAsset

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Group {
                switch asset.kind {
                case "video":
                    if let url = asset.url {
                        VideoPlayer(player: AVPlayer(url: url))
                    } else {
                        placeholder("video")
                    }
                case "audio":
                    audio
                default:
                    if let url = asset.url {
                        AsyncImage(url: url) { phase in
                            switch phase {
                            case .success(let image):
                                image.resizable().scaledToFill()
                            case .failure:
                                placeholder("photo.badge.exclamationmark")
                            default:
                                ZStack { Museum.surfaceElevated; ProgressView().tint(Museum.text) }
                            }
                        }
                    } else {
                        placeholder("photo")
                    }
                }
            }
            .frame(height: 210)
            .clipShape(RoundedRectangle(cornerRadius: Museum.Radius.lg, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Museum.Radius.lg, style: .continuous).stroke(Museum.border, lineWidth: 1))

            if !asset.caption.isEmpty {
                Text(asset.caption)
                    .font(Museum.bodyFont(13))
                    .foregroundStyle(Museum.text.opacity(0.85))
                    .lineLimit(3)
            }
            Text(creditLine)
                .font(Museum.bodyFont(11))
                .foregroundStyle(Museum.faint)
                .lineLimit(2)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(asset.caption.isEmpty ? "Media" : asset.caption)
    }

    private var audio: some View {
        VStack(spacing: 8) {
            Image(systemName: "waveform")
                .font(.system(size: 34, weight: .light))
                .foregroundStyle(Museum.text)
            if let url = asset.url {
                Link("Play audio", destination: url)
                    .font(Museum.bodyFont(13, weight: .semibold))
                    .foregroundStyle(Museum.blue)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Museum.surfaceElevated)
    }

    private func placeholder(_ symbol: String) -> some View {
        ZStack {
            Museum.accentGradient.opacity(0.6)
            Image(systemName: symbol)
                .font(.system(size: 38, weight: .ultraLight))
                .foregroundStyle(.white.opacity(0.8))
        }
    }

    private var creditLine: String {
        var parts: [String] = []
        if !asset.credit.isEmpty { parts.append(asset.credit) }
        if !asset.license.isEmpty { parts.append(asset.license) }
        return parts.isEmpty ? "Credit unavailable" : parts.joined(separator: " · ")
    }
}
