// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "PriceCore",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "PriceCore", targets: ["PriceCore"]),
        .executable(name: "PriceCoreCheck", targets: ["PriceCoreCheck"]),
        .executable(name: "PriceCoreLiveCheck", targets: ["PriceCoreLiveCheck"])
    ],
    targets: [
        .target(name: "PriceCore"),
        .executableTarget(name: "PriceCoreCheck", dependencies: ["PriceCore"]),
        .executableTarget(name: "PriceCoreLiveCheck", dependencies: ["PriceCore"])
    ]
)
