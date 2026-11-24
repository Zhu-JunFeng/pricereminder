import Foundation
import PriceCore
final class State: @unchecked Sendable {
 static let shared=State();let lock=NSLock();var reads=0;var saves=0;var locked=false;var missing=false;var denyInteraction=false;var interactiveReads=0
 func reset(locked:Bool=false,missing:Bool=false,denyInteraction:Bool=false){lock.lock();defer{lock.unlock()};reads=0;saves=0;self.locked=locked;self.missing=missing;self.denyInteraction=denyInteraction;interactiveReads=0}
}
enum KeychainStore {
 struct Failure:Error{}
 static func token(allowInteraction:Bool=false)throws->String?{
  let s=State.shared;s.lock.lock();defer{s.lock.unlock()};s.reads+=1
  if allowInteraction{s.interactiveReads+=1;if s.denyInteraction{throw Failure()}}
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
  // Explicit requests may all be waiting on the same failing background refresh.
  State.shared.reset();await TestAPI.shared.reset(failures:1)
  let overlapping=ServerConnectionService()
  let background=Task{try await overlapping.authenticatedClient(platform:"macos",displayName:"check")}
  try await Task.sleep(for:.milliseconds(5))
  try await withThrowingTaskGroup(of:Void.self){g in
   for _ in 0..<20 {g.addTask{_ = try await overlapping.authorize(platform:"macos",displayName:"check")}}
   try await g.waitForAll()
  }
  _ = try? await background.value
  precondition(State.shared.reads==2 && State.shared.interactiveReads==1)
  State.shared.reset(locked:true);await TestAPI.shared.reset()
  let denied=ServerConnectionService()
  for _ in 0..<10{do{_ = try await denied.authenticatedClient(platform:"macos",displayName:"check");fatalError()}catch{}}
  precondition(State.shared.reads==1)
  let registrations=await TestAPI.shared.registrations;precondition(registrations==0)
  try await withThrowingTaskGroup(of:Void.self){g in
   for _ in 0..<20 {g.addTask{_ = try await denied.authorize(platform:"macos",displayName:"check")}}
   try await g.waitForAll()
  }
  precondition(State.shared.reads==2 && State.shared.interactiveReads==1)
  // Once allowed, charts, directory and stream all reuse the authenticated client.
  for _ in 0..<10 {_ = try await denied.authenticatedClient(platform:"macos",displayName:"check")}
  precondition(State.shared.reads==2)
  State.shared.reset(locked:true,denyInteraction:true);await TestAPI.shared.reset()
  let cancelled=ServerConnectionService()
  do{_ = try await cancelled.authorize(platform:"macos",displayName:"check");fatalError()}catch{}
  for _ in 0..<10 {do{_ = try await cancelled.authenticatedClient(platform:"macos",displayName:"check");fatalError()}catch{}}
  precondition(State.shared.interactiveReads==1 && State.shared.reads==1)
  let createdAfterDenial=await TestAPI.shared.registrations;precondition(createdAfterDenial==0)
  State.shared.reset(missing:true);await TestAPI.shared.reset()
  let firstLaunch=ServerConnectionService()
  try await withThrowingTaskGroup(of:Void.self){g in
   for _ in 0..<20 {g.addTask{_ = try await firstLaunch.authorize(platform:"macos",displayName:"check")}}
   try await g.waitForAll()
  }
  precondition(State.shared.reads==1 && State.shared.saves==1)
  let createdOnce=await TestAPI.shared.registrations;precondition(createdOnce==1)
  print("PASS: concurrent requests coalesced; network retry reuses credentials; denied keychain does not retry or register; explicit authorization recovers")
 }
}
