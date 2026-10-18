const fs=require('node:fs');
const path=require('node:path');
const sharp=require(process.env.SHARP_MODULE||'sharp');
(async()=>{
 const root=__dirname;
 for(const size of [64,128,256,512])await sharp(path.join(root,'my-hosting.svg')).resize(size,size).png().toFile(path.join(root,`my-hosting-${size}.png`));
 const sizes=[16,24,32,48,64,128,256];
 const images=await Promise.all(sizes.map(size=>sharp(path.join(root,'my-hosting.svg')).resize(size,size).png().toBuffer()));
 const header=Buffer.alloc(6+16*images.length);header.writeUInt16LE(1,2);header.writeUInt16LE(images.length,4);let offset=header.length;
 images.forEach((image,i)=>{const p=6+16*i;header[p]=sizes[i]===256?0:sizes[i];header[p+1]=header[p];header.writeUInt16LE(1,p+4);header.writeUInt16LE(32,p+6);header.writeUInt32LE(image.length,p+8);header.writeUInt32LE(offset,p+12);offset+=image.length;});
 fs.writeFileSync(path.join(root,'my-hosting.ico'),Buffer.concat([header,...images]));
 console.log('Exported transparent PNG and Windows ICO variants');
})().catch(error=>{console.error(error);process.exitCode=1});
