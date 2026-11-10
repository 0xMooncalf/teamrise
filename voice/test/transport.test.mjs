import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createAudioResource,StreamType,generateDependencyReport} from '@discordjs/voice';
import OpusScript from 'opusscript';
import {Readable} from 'node:stream';

test('DAVE and local raw-to-Opus playback are available without FFmpeg',async()=>{
 assert.match(generateDependencyReport(),/@snazzah\/davey: 0\./);
 const pcm=Buffer.alloc(3840*10);
 const resource=createAudioResource(Readable.from([pcm]),{inputType:StreamType.Raw});
 const decoder=new OpusScript(48000,2,OpusScript.Application.AUDIO);
 let packets=0,samples=0;
 try {for await (const packet of resource.playStream){packets++;samples+=decoder.decode(packet).length;}}
 finally{decoder.delete();}
 assert.ok(packets>=10);assert.ok(samples>=pcm.length);
});
