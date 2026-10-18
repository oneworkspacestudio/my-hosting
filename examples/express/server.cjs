const express = require('express');
const app = express();
app.get('/healthz', (_request, response) => response.json({status: 'ok'}));
app.get('/', (_request, response) => response.type('html').send(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>My Hosting · Express</title><style>body{font:18px system-ui;background:#f5f7fc;color:#18233b;max-width:760px;margin:12vh auto;padding:24px}h1{font-size:48px;letter-spacing:-2px}.tag{color:#087f6c}article{padding:40px;background:white;border:1px solid #e3e8f4;border-radius:24px}</style><article><p class="tag">ONE WORKSPACE · MY HOSTING</p><h1>Your Node app is live.</h1><p>Real Express, deployed from a pinned GitHub source revision and served through Caddy.</p><p>Server-rendered at <strong>${new Date().toISOString()}</strong></p></article></html>`));
app.listen(Number(process.env.PORT), '127.0.0.1');
