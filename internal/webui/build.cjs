// Rebuild checked-in browser assets. Go builds consume dist without Node.js.
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const root = __dirname;
const assets = path.join(root, 'dist', 'assets');
fs.mkdirSync(assets, { recursive: true });
fs.copyFileSync(require.resolve('vue/dist/vue.global.prod.js'), path.join(assets, 'vue.global.prod.js'));
fs.copyFileSync(path.join(root, 'node_modules/vue/LICENSE'), path.join(assets, 'vue.LICENSE'));
fs.copyFileSync(path.join(root, 'node_modules/tailwindcss/LICENSE'), path.join(assets, 'tailwindcss.LICENSE'));
execFileSync(process.execPath, [require.resolve('tailwindcss/lib/cli.js'),
  '-i', path.join(root, 'styles.css'), '-o', path.join(assets, 'admin.css'),
  '--content', path.join(root, 'dist', 'index.html'), '--minify'], { cwd: root, stdio: 'inherit' });
