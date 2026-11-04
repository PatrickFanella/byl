// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "InvisibleMuseumKit",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "InvisibleMuseumKit", targets: ["InvisibleMuseumKit"])
    ],
    targets: [
        .target(name: "InvisibleMuseumKit"),
        .testTarget(
            name: "InvisibleMuseumKitTests",
            dependencies: ["InvisibleMuseumKit"],
            resources: [.copy("Fixtures")]
        )
    ]
)
