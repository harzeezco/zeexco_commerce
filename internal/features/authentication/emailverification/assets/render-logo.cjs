// Run with node; the optional argument is the installed @resvg/resvg-wasm directory.
const fs = require('node:fs');
const path = require('node:path');
const modulePath = process.argv[2] || '@resvg/resvg-wasm';
const { Resvg, initWasm } = require(modulePath);
(async () => {
  await initWasm(fs.readFileSync(require.resolve(modulePath + '/index_bg.wasm')));
  const svg = fs.readFileSync(path.join(__dirname, 'zico-logo.svg'), 'utf8')
    .replace('width="335" height="204" viewBox="0 0 335 204"',
      'width="328" height="120" viewBox="0 44 328 120"');
  const renderer = new Resvg(svg, { fitTo: { mode: 'width', value: 328 } });
  fs.writeFileSync(path.join(__dirname, 'zico-logo.png'), renderer.render().asPng());
})().catch(error => { console.error(error); process.exit(1); });
