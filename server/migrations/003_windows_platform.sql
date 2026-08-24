ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_platform_check;
ALTER TABLE devices
    ADD CONSTRAINT devices_platform_check
    CHECK (platform IN ('ios', 'android', 'macos', 'windows'));
