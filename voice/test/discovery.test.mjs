import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createSocket} from 'node:dgram';
import {retryIPDiscovery} from '../discovery.mjs';

test('IP discovery survives two lost UDP packets and stops retrying after ready',async()=>{
 const server=createSocket('udp4'),client=createSocket('udp4');
 await new Promise(resolve=>server.bind(0,'127.0.0.1',resolve));
 let received=0,retries=0;
 const state={status:'connecting',networking:{state:{code:2,connectionData:{ssrc:12345},udp:{send(packet){client.send(packet,server.address().port,'127.0.0.1');}}}}};
 let stop;
 try {
  await new Promise((resolve,reject)=>{
   const deadline=setTimeout(()=>reject(Error('discovery retries did not arrive')),2000);
   server.on('message',packet=>{
    try{assert.equal(packet.length,74);assert.equal(packet.readUInt16BE(0),1);assert.equal(packet.readUInt16BE(2),70);assert.equal(packet.readUInt32BE(4),12345);}catch(e){clearTimeout(deadline);reject(e);return;}
    if(++received===3){state.status='ready';clearTimeout(deadline);resolve();}
   });
   stop=retryIPDiscovery({get state(){return state;}},{intervalMs:15,limit:7,onRetry:n=>{retries=n;}});
  });
  await new Promise(resolve=>setTimeout(resolve,60));
  assert.equal(received,3);assert.equal(retries,3);
 } finally {stop?.();server.close();client.close();}
});
