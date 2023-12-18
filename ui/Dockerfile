FROM node:16

WORKDIR /usr/src/app

COPY package.json .
RUN npm install

COPY . .

RUN npm build

EXPOSE 3500
CMD ["npm", "start"]
