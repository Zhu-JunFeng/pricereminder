// npm install --prefix /tmp/pricereminder-icon-tools sharp
// NODE_PATH=/tmp/pricereminder-icon-tools/node_modules node apple/Brand/generate-icons.cjs
const fs = require('node:fs');
const path = require('node:path');
const sharp = require('sharp');
const root = path.resolve(__dirname, '../..');
const source = fs.readFileSync(path.join(__dirname, 'AppIcon.svg'), 'utf8');
async function main() {
  const assets = path.join(root, 'apple/Shared/Assets.xcassets/AppIcon.appiconset');
  for (const size of [16, 32, 64, 128, 256, 512, 1024]) {
    await sharp(Buffer.from(source), {density: 192}).resize(size, size).png().toFile(path.join(assets, `AppIcon-${size}.png`));
  }
  const ios = source.replace('x="64" y="64" width="896" height="896" rx="200"', 'x="0" y="0" width="1024" height="1024" rx="0"');
  await sharp(Buffer.from(ios)).flatten({background: '#091525'}).png().toFile(path.join(assets, 'AppIcon-iOS.png'));
  const sizes = [16, 24, 32, 48, 64, 128, 256];
  const buffers = await Promise.all(sizes.map(size => sharp(Buffer.from(source), {density: 192}).resize(size, size).png().toBuffer()));
  const header = Buffer.alloc(6 + buffers.length * 16);
  header.writeUInt16LE(1, 2); header.writeUInt16LE(buffers.length, 4);
  let offset = header.length;
  sizes.forEach((size, i) => {
    const p = 6 + i * 16;
    header[p] = header[p+1] = size === 256 ? 0 : size;
    header.writeUInt16LE(1, p+4); header.writeUInt16LE(32, p+6);
    header.writeUInt32LE(buffers[i].length, p+8); header.writeUInt32LE(offset, p+12);
    offset += buffers[i].length;
  });
  fs.writeFileSync(path.join(root, 'windows/PriceReminder.ico'), Buffer.concat([header, ...buffers]));
  fs.writeFileSync(path.join(root, 'android/app/src/main/res/drawable/ic_launcher_foreground.xml'), `<vector xmlns:android="http://schemas.android.com/apk/res/android" android:width="108dp" android:height="108dp" android:viewportWidth="1024" android:viewportHeight="1024">
    <group android:pivotX="512" android:pivotY="512" android:scaleX="0.8" android:scaleY="0.8">
        <path android:pathData="M264,628 L420,450 L552,556 L760,336" android:fillColor="@android:color/transparent" android:strokeColor="#62E4CB" android:strokeWidth="76" android:strokeLineCap="round" android:strokeLineJoin="round"/>
        <path android:pathData="M712,336 a48,48 0,1 0,96 0 a48,48 0,1 0,-96 0" android:fillColor="#79F3D2"/>
    </group>
</vector>
`);
}
main().catch(error => { console.error(error); process.exit(1); });
