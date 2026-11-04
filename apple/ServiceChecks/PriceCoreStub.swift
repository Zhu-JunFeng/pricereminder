import Foundation
public struct Contract: Sendable {}
public enum NetworkFailure: Error { case offline }
public actor TestAPI {
 public static let shared=TestAPI()
 public var refreshes=0
 public var registrations=0
 public var failures=0
 public func reset(failures:Int=0){refreshes=0;registrations=0;self.failures=failures}
 public func refresh() throws -> Bool {refreshes+=1;if failures>0{failures-=1;throw NetworkFailure.offline};return true}
 public func register(){registrations+=1}
}
public actor APIClient {
 public struct Registration: Sendable {public let token:String}
 private var token:String?
 public init(baseURL:URL,token:String?=nil){self.token=token}
 public func refreshDevice() async throws -> Bool {try await Task.sleep(for:.milliseconds(30));return try await TestAPI.shared.refresh()}
 public func register(platform:String,displayName:String)async throws->Registration{await TestAPI.shared.register();return Registration(token:"new")}
 public func setToken(_ token:String){self.token=token}
 public func contracts()->[Contract]{[]}
 public func setSubscriptions(_ s:[String]) throws {}
 public func streamURL() throws ->URL{URL(string:"wss://example.invalid")!}
 public func bearerToken() throws ->String{token!}
}
