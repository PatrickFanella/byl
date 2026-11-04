import Foundation

enum SampleData {
    static let spaces: [MuseumSpace] = [
        MuseumSpace(
            id: "sp-station",
            name: "The Ghost Station",
            tagline: "Listen to a transit line that vanished in 1924",
            summary: "A forgotten platform under the square returns as a spatial archive of departures, brass signs, worker stories, and community recordings.",
            latitude: 40.7505, longitude: -73.9934, radiusMeters: 42, distanceMeters: 120,
            kind: .station, exhibitCount: 5, contributorCount: 41, accentHue: 0.55, isDownloaded: true,
            exhibits: [
                Exhibit(id: "st-1", title: "Announcements, 1962", kind: .audio, body: "The station's original tannoy voice, restored from a single surviving reel. Close your eyes and the platform fills again.", year: "1962", contributor: "Sound Archive", tags: ["audio", "transit"], status: .published, confidence: .high, offset: SIMD3<Float>(0, 1.4, -2), durationSeconds: 54, likes: 156),
                Exhibit(id: "st-2", title: "The Departure Board", kind: .artifact, body: "A working reconstruction of the mechanical split-flap board, clattering through destinations that no longer run.", year: "1948", contributor: "Reconstruction Guild", tags: ["3D", "mechanism"], status: .published, confidence: .high, offset: SIMD3<Float>(1.8, 0.8, -2.6), durationSeconds: nil, likes: 203),
                Exhibit(id: "st-3", title: "Platform Zero", kind: .image, body: "A night photograph found in a retired conductor's suitcase, showing the platform before it was sealed.", year: "1924", contributor: "City Archive", tags: ["photo", "archive"], status: .published, confidence: .medium, offset: SIMD3<Float>(-1.2, 1, -1.7), durationSeconds: nil, likes: 88)
            ]
        ),
        MuseumSpace(
            id: "sp-storefront",
            name: "Lost Corner Store",
            tagline: "A neighborhood market rebuilt from memory",
            summary: "Residents contributed receipts, snapshots, shelf labels, and voice notes to reconstruct the store that anchored this intersection before redevelopment.",
            latitude: 40.7041, longitude: -74.0125, radiusMeters: 58, distanceMeters: 340,
            kind: .storefront, exhibitCount: 7, contributorCount: 28, accentHue: 0.25, isDownloaded: false,
            exhibits: [
                Exhibit(id: "co-1", title: "1980s Neon Signs", kind: .image, body: "Photographs of the storefront signs collected from family albums and local newspaper scans.", year: "1980–1989", contributor: "Local Residents", tags: ["neon", "photo"], status: .published, confidence: .high, offset: SIMD3<Float>(0.8, 1.2, -2.2), durationSeconds: nil, likes: 94),
                Exhibit(id: "co-2", title: "Saturday Receipts", kind: .story, body: "A small pile of receipts reveals the rhythms of weekend shopping: milk, batteries, flowers, coffee, and lottery tickets.", year: "1991", contributor: "Mina Park", tags: ["story"], status: .published, confidence: .medium, offset: SIMD3<Float>(-1.6, 0.5, -1.6), durationSeconds: nil, likes: 61)
            ]
        ),
        MuseumSpace(
            id: "sp-library",
            name: "The First Library",
            tagline: "A civic room remembered by readers",
            summary: "A quiet gallery tracing the first lending library, from handwritten cards to oral histories from people who learned to read here.",
            latitude: 40.7308, longitude: -73.9975, radiusMeters: 50, distanceMeters: 720,
            kind: .hall, exhibitCount: 4, contributorCount: 17, accentHue: 0.62, isDownloaded: false,
            exhibits: [
                Exhibit(id: "li-1", title: "Cards in the Drawer", kind: .timeline, body: "A century of library cards, animated into a walking timeline around the original reading room footprint.", year: "1902–2002", contributor: "Public Library", tags: ["timeline"], status: .underReview, confidence: .medium, offset: SIMD3<Float>(0, 1, -2.8), durationSeconds: nil, likes: 42)
            ]
        )
    ]

    static let moderationQueue: [ContributionModerationItem] = [
        ContributionModerationItem(
            id: "contrib-101",
            spaceID: "sp-station",
            suggestedTitle: "Platform timetable photo",
            kind: .image,
            body: "A snapshot of the old timetable board before the station renovation.",
            contributorName: "Mina Park",
            status: .pending,
            createdAt: .now.addingTimeInterval(-3_600),
            updatedAt: .now.addingTimeInterval(-3_600)
        ),
        ContributionModerationItem(
            id: "contrib-102",
            spaceID: "sp-library",
            suggestedTitle: "Reading room oral history",
            kind: .story,
            body: "A short memory from someone who learned to read in the original library annex.",
            contributorName: "Alex Curator",
            status: .pending,
            createdAt: .now.addingTimeInterval(-7_200),
            updatedAt: .now.addingTimeInterval(-7_200)
        )
    ]
}
