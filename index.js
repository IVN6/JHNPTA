require("dotenv").config()


/*const http = require('http');

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/plain' });
  res.end('Hello, World!\n');
});

server.listen(PORT, () => {
    console.log(`Server is running on port ${PORT}`);
});*/

/*

const express = require('express')
const app = express()
const port = process.env.PORT || 4000;


//servir archivos estaticos
app.use(express.static('js','css','index.html'))


app.get('/', (req, res) => { 
  res.send('Hello World!')//here has to be placed the files that we want to be shown 
})


//escuchar reuests
app.listen(port, () => {
  console.log(`Example app listening on port ${port}`)
})*/