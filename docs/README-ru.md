<p align="center">
  <a href="https://github.com/aliking1367/next" target="_blank" rel="noopener noreferrer">
    <img width="160" height="160" src="../dashboard/src/assets/logo.svg" alt="Next logo">
  </a>
</p>

<h1 align="center"/>Next</h1>

<p align="center">
    Унифицированное решение с графическим интерфейсом, устойчивое к цензуре, на базе <a href="https://github.com/XTLS/Xray-core">Xray</a>
</p>

<br/>
<p align="center">
    <a href="#">
        <img src="https://img.shields.io/github/actions/workflow/status/aliking1367/next/build.yml?style=flat-square" />
    </a>
    <a href="https://hub.docker.com/r/aliking1367/next" target="_blank">
        <img src="https://img.shields.io/docker/pulls/aliking1367/next?style=flat-square&logo=docker" />
    </a>
    <a href="#">
        <img src="https://img.shields.io/github/license/aliking1367/next?style=flat-square" />
    </a>
    <a href="https://t.me/aliking1367_next" target="_blank">
        <img src="https://img.shields.io/badge/telegram-channel-blue?style=flat-square&logo=telegram" />
    </a>
    <a href="#">
        <img src="https://img.shields.io/github/stars/aliking1367/next?style=social" />
    </a>
</p>

<p align="center">
 <a href="../README.md">
 English
 </a>
 /
 <a href="./README-fa.md">
 فارسی
 </a>
  /
  <a href="./README-zh-cn.md">
 简体中文
 </a>
   /
  <a href="./README-ru.md">
 Русский
 </a>
</p>

## Оглавление

- [Введение](#введение)
  - [Почему Next?](#почему-next)
    - [Функции](#функции)
- [Руководство по установке](#руководство-по-установке)
- [Конфигурация](#конфигурация)
- [Telegram Bot](#telegram-bot)
- [Webhook уведомления](#webhook-уведомления)
- [Поддержка](#поддержка)
- [Лицензия](#лицензия)
- [Участники](#участники)

# Введение

Next — это инструмент управления прокси-серверами, который предоставляет простой и удобный пользовательский интерфейс для управления сотнями учетных записей прокси на базе [Xray-core](https://github.com/XTLS/Xray-core) и созданный на Go и ReactJS.

## Почему Next?

Next удобен в использовании, многофункционален и надежен. Он позволяет создавать различные прокси для пользователей без сложной настройки. С помощью встроенного веб-интерфейса можно контролировать, изменять и ограничивать пользователей.

### Функции

- Готовый **Web UI**
- **REST API** бэкэнд
- Поддержка [**множества узлов**](#next-node) (для распределения инфраструктуры и масштабируемости)
- Поддержка протоколов **Vmess**, **VLESS**, **Trojan** и **Shadowsocks**
- Возможность активации **нескольких протоколов** для каждого пользователя
- **Несколько пользователей** на одном inbound
- **Несколько inbound** на **одном порту** (поддержка fallbacks)
- Ограничения на основе **количества трафика** и **срока действия**
- Ограничение трафика по **периодам** (например выдавать трафик на день, неделю и т. д.)
- Поддержка **ссылок-подписок** совместимых с **V2ray** _(такие как V2RayNG, SingBox, Nekoray, и др.)_, **Clash** и **ClashMeta**
- Автоматическая генерация **Ссылок** и **QRcode** 
- Мониторинг ресурсов сервера и **использования трафика**
- Настраиваемые конфигурации xray
- Поддержка **TLS** и **REALITY** 
- Встроенный **Telegram Bot**
- Встроенный **Command Line Interface (CLI)**
- **Несколько языков**
- Поддержка **Нескольких администраторов** (WIP)

# Руководство по установке

Установите Next master через бинарный установщик:

```bash
curl -sL https://raw.githubusercontent.com/aliking1367/next/master/scripts/next/next-binary.sh | sudo bash -s -- install
```

Установите Next-node на каждом node-сервере через бинарный установщик node:

```bash
curl -sL https://raw.githubusercontent.com/aliking1367/next/master/scripts/next/next-node-binary.sh | sudo bash -s -- install
```

Бинарные установщики создают native systemd-сервисы и автоматически скачивают подходящий Linux binary для архитектуры сервера. Master поддерживает SQLite, MySQL и MariaDB через параметры установки; node-установщик устанавливает только runtime ноды и подключается к Master через certificate/token flow из панели.

Когда установка будет завершена:
- Вы увидите логи, которые можно остановить, нажав `Ctrl+C` или закрыв терминал.
- Файлы Next будут размещены по адресу `/opt/next`.
- Файл конфигурации будет размещен по адресу `/opt/next/.env` (см. [Конфигурация](#конфигурация)).
- Файлы с данными будут размещены по адресу `/var/lib/next`.
- По соображениям безопасности, панель управления Next недоступна через IP-адрес. Поэтому вам необходимо [получить SSL-сертификат](https://aliking1367.github.io/next/ru/examples/issue-ssl-certificate) и получить доступ к панели управления Next, открыв веб-браузер и перейдя по адресу `https://YOUR_DOMAIN:8000/dashboard/` (замените YOUR_DOMAIN на ваш фактический домен).
- Вы также можете использовать перенаправление портов SSH для локального доступа к панели управления Next без домена. Замените `user@serverip` на ваше фактическое имя пользователя SSH и IP-адрес сервера и выполните следующую команду:

```bash
ssh -L 8000:localhost:8000 user@serverip
```

Наконец, введите следующую ссылку в ваш браузер, чтобы получить доступ к панели управления Next:

http://localhost:8000/dashboard/

Вы потеряете доступ к панели управления, как только закроете терминал SSH. Поэтому этот метод рекомендуется использовать только для тестирования.

Далее, Вам нужно создать главного администратора для входа в панель управления Next, выполнив следующую команду: 

```bash
next cli admin create --sudo
```

Готово! Теперь Вы можете войти, используя данные своей учетной записи.

Для того, чтобы увидеть справочное сообщение от скрипта Next, выполните команду:

```bash
next --help
```

Если Вы хотите запустить проект, используя его исходный код, обратитесь к разделу ниже
<details markdown="1">
<summary><h3>Ручная установка</h3></summary>

Установите xray на Ваш сервер.

Вы можете сделать это, используя [Xray-install](https://github.com/XTLS/Xray-install):

```bash
curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh | bash -s -- install
```

Клонируйте проект и соберите dashboard и Go-бинарники:

```bash
git clone https://github.com/aliking1367/next.git
cd Next
cd dashboard
npm ci
VITE_BASE_API=/api/ npm run build -- --outDir=build --assetsDir=statics
cp ./build/index.html ./build/404.html
cd ..
bash scripts/build_binary.sh
```

Затем выполните следующую команду для запуска Go-миграций базы данных:

```bash
next migrate up
```

Если Вы хотите использовать `next-cli`, необходимо связать его с файлом в `$PATH`, сделать его исполняемым и установить:

```bash
sudo install -m 755 ./dist/next-cli /usr/local/bin/next
next cli --help

```

Теперь настало время настройки.

Создайте копию файла `.env.example`, посмотрите его и отредактируйте с помощью текстового редактора,например `nano`.

Возможно, вам захочется изменить учетные данные администратора.

```bash
cp .env.example .env
nano .env
```

> Проверьте раздел [Конфигурации](#конфигурация) для получения большей информации.

В завершение запустите приложение с помощью следующей команды:

```bash
./dist/next-server
```

Для ручной установки через systemd создайте unit для Go-бинарника:

```ini
[Unit]
Description=Next
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/next
EnvironmentFile=/opt/next/.env
ExecStart=/opt/next/dist/next-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Затем включите сервис:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now next
```

Для использования с nginx:

```
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name  example.com;

    ssl_certificate      /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key  /etc/letsencrypt/live/example.com/privkey.pem;

    location ~* /(dashboard|statics|sub|api|docs|redoc|openapi.json) {
        proxy_pass http://0.0.0.0:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # xray-core ws-path: /
    # client ws-path: /next/me/2087
    #
    # All traffic is proxed through port 443, and send to the xray port(2087, 2088 etc.).
    # The '/next' in location regex path can changed any characters by yourself.
    #
    # /${path}/${username}/${xray-port}
    location ~* /next/.+/(.+)$ {
        proxy_redirect off;
        proxy_pass http://127.0.0.1:$1/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

или:

```
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name  next.example.com;

    ssl_certificate      /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key  /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://0.0.0.0:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

По умолчанию приложение будет запускаться на `http://localhost:8000/dashboard`. Вы можете настроить его, изменив переменные окружения `UVICORN_HOST` и `UVICORN_PORT`.
</details>

# Конфигурация

> Ниже приведены настройки, которые можно задать с помощью переменных окружения поместив их в файл `.env`.

| Переменная                               | Описание                                                                                                      |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| SQLALCHEMY_DATABASE_URL                  | URL базы данных; legacy-имя сохранено для совместимости с Go runtime.                                         |
| UVICORN_HOST                             | Хост публичного Go gateway (по умолчанию: `0.0.0.0`).                                                        |
| UVICORN_PORT                             | Порт публичного Go gateway (по умолчанию: `8000`).                                                           |
| UVICORN_SSL_CERTFILE                     | TLS-сертификат для Go gateway.                                                                                |
| UVICORN_SSL_KEYFILE                      | TLS-ключ для Go gateway.                                                                                      |
| UVICORN_SSL_CA_TYPE                      | Тип CA для install scripts: `public` или `private`.                                                          |
| NEXT_GATEWAY_ADDR                     | Полный адрес gateway; переопределяет `UVICORN_HOST`/`UVICORN_PORT`.                                          |
| NEXT_NODE_OPERATIONS_POLL_INTERVAL    | Интервал обработки очереди node operations.                                                                   |
| NEXT_USER_LIFECYCLE_INTERVAL          | Интервал проверки lifecycle пользователей.                                                                    |
| NEXT_USER_USAGE_RESET_INTERVAL        | Интервал периодического reset usage пользователей.                                                            |
| USERS_AUTODELETE_DAYS                    | Удалять expired пользователей через это число дней; отрицательное значение отключает функцию.                 |
| USER_AUTODELETE_INCLUDE_LIMITED_ACCOUNTS | Включать limited пользователей в auto-delete.                                                                 |
| JWT_ACCESS_TOKEN_EXPIRE_MINUTES          | Время жизни JWT access token в минутах.                                                                       |
| USERS_LIST_TIMEOUT_SECONDS               | Таймаут больших user list запросов; `0` отключает таймаут.                                                    |
| NEXT_CERT_BASE                        | Базовый путь управляемых сертификатов.                                                                        |
| NEXT_CONFIG_DIR                       | Корень конфигурации, включаемый в full backup.                                                                |

# Telegram Bot

Next поставляется с встроенным ботом Telegram, который может управлять сервером, создавать и удалять пользователей, а также отправлять уведомления. Этот бот можно легко включить, выполнив несколько простых шагов, и он предоставляет удобный способ взаимодействия с Next без необходимости каждый раз заходить на сервер.

Чтобы включить Telegram-бота, выполните следующие действия:

1. установите `TELEGRAM_API_TOKEN` в качестве API-токена вашего бота.
2. установите `TELEGRAM_ADMIN_ID` в качестве цифрового ID вашего Telegram-аккаунта, который вы можете получить от [@userinfobot](https://t.me/userinfobot)

Команды Telegram-бота, отчёты и доставка резервных копий поддерживаются Go-сервисами и настраиваются в панели.

# Webhook уведомления

Вы можете задать адрес webhook, и Next будет отправлять уведомления на этот адрес.

Запросы будут отправляться в виде POST-запроса на адрес, указанный в `WEBHOOK_ADDRESS`, с `WEBHOOK_SECRET` в качестве `x-webhook-secret` в заголовках.

Пример запроса, отправленного Next:

```
Headers:
Host: 0.0.0.0:9000
User-Agent: python-requests/2.28.1
Accept-Encoding: gzip, deflate
Accept: */*
Connection: keep-alive
x-webhook-secret: something-very-very-secret
Content-Length: 107
Content-Type: application/json



Body:
{"username": "next_test_user", "action": "user_updated", "enqueued_at": 1680506457.636369, "tries": 0}
```

Различные типы действий: `user_created`, `user_updated`, `user_deleted`, `user_limited`, `user_expired`, `user_disabled`, `user_enabled`

# Поддержка

Если вы нашли Next полезным и хотели бы поддержать его развитие, вы можете сделать пожертвование в одной из следующих криптовалютных сетей:

- TRON network (TRC20): `TGftLESDAeRncE7yMAHrTUCsixuUwPc6qp`
- ETH, BNB, MATIC network (ERC20, BEP20): `0x413eb47C430a3eb0E4262f267C1AE020E0C7F84D`
- TON network: `UQDNpA3SlFMorlrCJJcqQjix93ijJfhAwIxnbTwZTLiHZ0Xa`

Спасибо за поддержку!

# Лицензия

Сделано в [Unknown!] и опубликовано под [AGPL-3.0](./LICENSE).

# Участники

Мы ❤️‍🔥 участников проекта! Если вы хотите внести свой вклад, пожалуйста, ознакомьтесь с нашим [Contributing Guidelines](CONTRIBUTING.md) и не стесняйтесь отправлять запросы на исправление ошибок или сообщить о проблеме. Мы также приглашаем вас присоединиться к нашей группе [Telegram](https://t.me/aliking1367_next) для получения поддержки.

Проверьте [open issues](https://github.com/aliking1367/next/issues), чтобы помочь развитию этого проекта.

<p align="center">
Спасибо всем участникам, благодаря которым Next становится лучше:
</p>
<p align="center">
<a href="https://github.com/aliking1367/next/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=aliking1367/next" />
</a>
</p>
<p align="center">
  Made with <a rel="noopener noreferrer" target="_blank" href="https://contrib.rocks">contrib.rocks</a>
</p>
