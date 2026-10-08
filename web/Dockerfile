FROM node:20-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .

EXPOSE 5174

# Запускаем dev-сервер на порту 5174 и открываем его для Docker-сети
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5174"]
