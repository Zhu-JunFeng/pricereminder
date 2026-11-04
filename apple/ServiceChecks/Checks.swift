import Foundation
import PriceCore
final class State: @unchecked Sendable {
 static let shared=State();let lock=NSLock();var reads=0;var saves=0;var locked=false;var missing=false
 func reset(locked:Bool=false,missing:Bool=false){lock.lock();defer{lock.unlock()};reads=0;saves=0;self.locked=locked;self.missing=missing}
}
enum KeychainStore {
 struct Failure:Error{}
 static func token(allowInteraction:Bool=false)throws->String?{
  let s=State.shared;s.lock.lock();defer{s.lock.unlock()};s.reads+=1
  if s.locked && !allowInteraction{throw Failure()};return s.missing ? nil : "existing"
 }
 static func save(token:String,allowInteraction:Bool=false)throws {
  let s=State.shared;s.lock.lock();defer{s.lock.unlock()};s.saves+=1
  if s.locked && !allowInteraction{throw Failure()}
 }
}
@main struct Checks {
 static func main()async throws {
  State.shared.reset();await TestAPI.shared.reset()
  let service=ServerConnectionService()
  try await withThrowingTaskGroup(of:Void.self){g in for _ in 0..<20{g.addTask{_ = try await service.authenticatedClient(platform:"macos",displayName:"check")}};try await g.waitForAll()}
  precondition(State.shared.reads==1);let refreshed=await TestAPI.shared.refreshes;precondition(refreshed==1)
  State.shared.reset();await TestAPI.shared.reset(failures:1)
  let retry=ServerConnectionService()
  do{_ = try await retry.authenticatedClient(platform:"macos",displayName:"check");fatalError()}catch{}
  _ = try await retry.authenticatedClient(platform:"macos",displayName:"check")
  precondition(State.shared.reads==1)
  State.shared.reset(locked:true);await TestAPI.shared.reset()
  let denied=ServerConnectionService()
  for _ in 0..<10{do{_ = try await denied.authenticatedClient(platform:"macos",displayName:"check");fatalError()}catch{}}
  precondition(State.shared.reads==1)
  let registrations=await TestAPI.shared.registrations;precondition(registrations==0)
  _ = try await denied.authorize(platform:"macos",displayName:"check")
  precondition(State.shared.reads==2)
  print("PASS: concurrent requests coalesced; network retry reuses credentials; denied keychain does not retry or register; explicit authorization recovers")
 }
}
