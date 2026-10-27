import http from 'node:http';
import {readFile,stat} from 'node:fs/promises';
import path from 'node:path';
const base=process.cwd();
const types={'.html':'text/html; charset=utf-8','.css':'text/css; charset=utf-8','.js':'text/javascript; charset=utf-8','.jpg':'image/jpeg','.png':'image/png','.md':'text/plain; charset=utf-8'};
const server=http.createServer(async(req,res)=>{try{let rel=decodeURIComponent(new URL(req.url,'http://localhost').pathname);if(rel==='/')rel='/prototype/';let file=path.resolve(base,'.'+rel);if(!file.startsWith(base+path.sep)){res.writeHead(403);res.end();return;}if((await stat(file)).isDirectory())file=path.join(file,'index.html');const bytes=await readFile(file);res.writeHead(200,{'Content-Type':types[path.extname(file)]||'application/octet-stream','Cache-Control':'no-cache'});res.end(req.method==='HEAD'?undefined:bytes);}catch{res.writeHead(404);res.end('Not found');}});
server.listen(5192,'127.0.0.1',()=>console.log('Preview: http://127.0.0.1:5192/prototype/'));
