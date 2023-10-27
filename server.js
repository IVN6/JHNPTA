/*const http = require('http');

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/plain' });
  res.end('Hello, World!\n');
});

server.listen(PORT, () => {
  console.log(`Server is running on port ${PORT}`);
});*/

const path = require('path')
require('dotenv').config({ path: path.resolve(__dirname, '../.env') })

const PORT = process.env.PORT || 3000;
const express = require('express')
const app = express()


//servir archivos estaticos
app.use(express.static('public'))


app.get('/', (req, res) => { 
  res.sendFile('public/index.html')//here has to be placed the files that we want to be shown 
})
app.get('/landing', (req, res) => { 
  res.send({name:"you" },{msj: "welcome"})//here has to be placed the files that we want to be shown 
})
app.get('/', (req, res) => { 
  res.send({name:"ybuto" },{msj: "welcome"})//here has to be placed the files that we want to be shown 
})



//escuchar reuests
app.listen(PORT, () => {
  console.log(`Example app listening on port ${PORT}`)
})