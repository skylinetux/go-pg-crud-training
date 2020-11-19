# Мониторинг: Prometheus, Grafana, Alertmanager

Цель данного проекта в том, чтобы показать как работают и взаимодействуют Prometheus, Grafana и Alertmanager. Если всё пойдёт хорошо, то в результате получим работающую систему мониторинга с оповещениями пользователей в Telegram.

## Обзор окружения

Для организации примера мониторинга приложения развёрнуто тоже окружение, что и в [примере с CI\\CD](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd). В данном случае нам понадобится **кластер OpenShift**, [приложение](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd/application) которое будем заводить в мониторинг, и хост **openshift-build**, которого будем производить установку и настройку стека мониторинга.

* Кластер **OpenShift**
* ВМ **openshift-infra**
  - Postgres
* ВМ **openshift-build**
  - git
  - vi\\vim
  - oc
  - kustomize
  - docker

#### ВМ **openshift-infra**

**Роль:** Запущены сервисы Gitlab CI, Nexus, Postgres. В данном примере используется только БД Postgres для корректной работы [приложения](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd/application).

Сервисы запущены в docker.

#### **Кластер OpenShift**

```console
# получить список нод и их статус
$ oc get nodes
NAME                              STATUS   ROLES            AGE   VERSION
master-0.ocp-test.<domain_name>   Ready    master,worker    68d   v1.18.3+45b9524
master-1.ocp-test.<domain_name>   Ready    master,worker    68d   v1.18.3+45b9524
master-2.ocp-test.<domain_name>   Ready    master,worker    68d   v1.18.3+45b9524
worker-0.ocp-test.<domain_name>   Ready    compute,worker   68d   v1.18.3+45b9524
worker-1.ocp-test.<domain_name>   Ready    compute,worker   68d   v1.18.3+45b9524

# получить версию кластера
$ oc get clusterversion
NAME      VERSION   AVAILABLE   PROGRESSING   SINCE   STATUS
version   4.5.17    True        False         8d      Cluster version is 4.5.17
```

#### Postgress

Приложение работает с БД, поэтому отдельно запущен Postgres на хосте openshift-infra.

Создана база данных **books_database** и таблица books.

#### ВМ openshift-build

Установлены утилиты oc, git, vim, kustomize

* yum -y install git vim go
* https://kubectl.docs.kubernetes.io/installation/kustomize/
* https://docs.docker.com/engine/install/centos/

## Общая схема работы мониторинга

![](/monitoring/images/img_1.png)

* **Prometheus server** выступает в роли хранилища метрик, которые мы будем получать из приложения. Также в Promethes server будем генерировать события для Alertmanager.
* В **Grafana** будем создавать дашборды для визуализации метрик.
* **Alertmanager** работает в качестве системы оповещения. Получая события из Prometheus, обрабытывая их, Alertmanager будет передавать данные в Alertmanager-bot.
* **Alertmanager-bot** система отправки оповещений в Telegram. Проект и описание доступны по сслыке [github.com/metalmatze/alertmanager-bot](https://github.com/metalmatze/alertmanager-bot).
* **Метрики приложения** доступны по ссылке http://go-pg-crud.go-pg-crud.svc:80/metrics. Для этого в приложение добавлен [экспортёр для golang](https://prometheus.io/docs/guides/go-application/):

```golang
...
import (
...
        "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
...
         servermain.Handle("/metrics", promhttp.Handler())

...
}
````

## Подготовка окружения для в OpenShift

Для начала необходимо подготовить окружение для запуска системы мониторинга. Необходимо:
* Создать namespace
* Создать учётную запись с правами доступа к метрикам приложений

Создаём namespace с именем training-monitoring и serviceaccount(учётная запись) с именем metricsexporter. Для metricsexporter предоставляем роли cluster-reader и view для доступа к метрикам сервисов. Под учётной записью metricsexporter будет запущен только pod с prometheus server. Всё остальные pod будут использовать учётную запись default.

```console
# переходим в директорию namespace
$ cd namespace

# с помощью kustomize генерируем конфигурацию и применяем
$ kustomize build . | oc apply -f -
namespace/training-monitoring created
serviceaccount/metricsexporter created
clusterrolebinding.rbac.authorization.k8s.io/cluster-reader-0 unchanged
clusterrolebinding.rbac.authorization.k8s.io/view unchanged

# проверяем наличие учётной записи metricsexporter в namespace training-monitoring
$ oc get sa -n training-monitoring
NAME              SECRETS   AGE
builder           2         50s
default           2         50s
deployer          2         50s
metricsexporter   2         50s

# проверяем наличие ролей для учётной записи training-monitoring
$ oc get clusterrolebinding -o wide | grep metricsexporter
cluster-reader-0  ClusterRole/cluster-reader    2d7h    training-monitoring/metricsexporter
view    ClusterRole/view    2d7h    training-monitoring/metricsexporter
```

## Prometheus server

**Prometheus server** центральный компонент системы мониторинга. Его задача хранение и получение обьектов мониторинга - метрик. Он использует базу данных TSDB (time series database) и хранит данные в виде временных рядов — наборов значений, соотнесённых с временной меткой (timestamp). Более подробно о TSDB и как работает Prometheus можно ознакомиться [на habr](https://habr.com/ru/company/southbridge/blog/455290/) и [на официальном сайте](https://prometheus.io/docs/prometheus/latest/storage/).

Для запуска prometheus server создать deploymentconfig, в котором будет запущен образ docker.io/prom/prometheus:latest, и конфигурационные файлы:
* prometheus.yml - основной файл конфигурации prometheus server
* go-pg-crud-rules.yaml - файл настройки правил

**prometheus.yml**

```yaml
# глобальные параметры
global:
  scrape_interval:     15s # частота сборки метрик
  evaluation_interval: 15s # частота оценки\проверки правил

# путь для правил
rule_files:
  - /etc/prometheus/rules/*.yaml

# блок настройки сборщиков метрик
scrape_configs:
  - job_name: go-pg-crud # имя job для сборки метрик
    static_configs:
      - targets: ['go-pg-crud.go-pg-crud.svc:80'] # адрес приложения по которому доступны метрики, по умолчанию добавляется /metrics к url

# блок настройки взаимодействия с Alertmanager
alerting:
  alertmanagers:
    - static_configs:
      - targets:
        - alertmanager:9093 # адрес alertmanager, куда prometheus будет отправлять события
```

**go-pg-crud-rules.yaml**

```yaml
groups:
  - name: go-pg-crud # имя группы парвил
    rules:
      - alert: HighGoroutine # имя alert
        expr: go_goroutines{job="go-pg-crud"} > 100 # собственно правило
        for: 1m # интервал срабатывания, т.е. в течении 1 минуты правило становится активным (FIRING)
        labels: # метки
          severity: page
        annotations: # описание правила и сообщения при срабатывании
          summary: High goroutine!
          message: Check server load

```

Важно отметить, что адрес сервиса alertmanager:9093 указан по имени service, аналогично для go-pg-crud.go-pg-crud.svc:80 - serivce с именем go-pg-crud в namespace go-pg-crud. Доступ к service в других namespace мы предоставили выше для учётной записи metricsexporter, когда предоставили роли cluster-reader и view. 
Обращение через service в нашем случае удобнее - мы собираем метрики внутри сети OpenShift, но возможен и вариант указания route необходимых сервисов. При настройке через route обращения к метрикам будут дополнительно проходить через балансировщик OpenShift, что не совсем рационально. Для сервисов вне OpenShift настройка сбора метрик через внешний адрес является основным.

Также создаём route и service для доступа к prometheus server. Полностью создание prometheus service в OpenShift будет выглядеть следующим образом:

```console
# переходим в директорию namespace
$ cd prometheus

# с помощью kustomize генерируем конфигурацию и применяем
$ kustomize build . | oc apply -f -
configmap/prometheus-config created
configmap/prometheus-rules created
service/prometheus created
deploymentconfig.apps.openshift.io/prometheus created
route.route.openshift.io/prometheus created

# проверяем наличе запущенных pod
$ oc get pod
NAME                  READY   STATUS              RESTARTS   AGE
prometheus-1-deploy   1/1     Running             0          4s
prometheus-1-pm2tn    0/1     ContainerCreating   0          2s

# проверяем наличие configmaps
$ oc get cm
NAME                DATA   AGE
prometheus-config   1      13m
prometheus-rules    1      13m

# проверяем наличие route
$ oc get route
NAME         HOST/PORT                                                    PATH   SERVICES     PORT   TERMINATION   WILDCARD
prometheus   prometheus-training-monitoring.apps.ocp-test.<domain_name>         prometheus   9090                 None

# проверяем наличие service
$ oc get svc
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
prometheus   ClusterIP   172.30.136.92   <none>        9090/TCP   13m
```

Сервис prometheus будет доступен по адресу http://prometheus-training-monitoring.apps.ocp-test.<domain_name>

## Grafana

**Grafana** - это платформа с открытым исходным кодом для визуализации, мониторинга и анализа данных. 

Для запуска grafana необходимо также создать deploymentconfig, в котором будет запущен образ docker.io/grafana/grafana, и конфигурационные файлы:
* grafana.ini - основной файл конфигурации grafana
* go-pg-crud_dashboard.json - заранее подготовленный dashboard для мониторинга приложения на golang, за основу взят [dashboard с сайта grafana](https://grafana.com/grafana/dashboards/6671)
* dashboards.yaml - верхнеуровневый dashboard, в котором указываем путь для других dashboard
* datasources.yaml - файл конфигурации подключений к prometheus server или другому источнику данных

**grafana.ini**

```yaml
[auth.basic]
enabled = false # отключаем basic аутентификацию

[auth.anonymous]
enabled = true # разрешаем anonymous доступ к интерфейсу grafana и задаём по умолчанию организацию и роль
org_name = Main Org.
org_role = Viewer

# блок описания путей для хранения данных, логов, плагинов и т.д. в grafana
[paths]
data = /var/lib/grafana
logs = /var/lib/grafana/logs
plugins = /var/lib/grafana/plugins
provisioning = /etc/grafana/provisioning

[server]
http_port = 3000 # grafana будет слушать порт 3000
```

**dashboards.yaml**

```json
{
    "apiVersion": 1,
    "providers": [
        {
            "folder": "",
            "name": "0",
            "options": { /* путь хранения дашбордов, которые будут использоваться как дочерние */
                "path": "/grafana-dashboard-definitions/"
            },
            "orgId": 1,
            "type": "file"
        }
    ]
}
```

**datasources.yaml**

```json
{
    "apiVersion": 1,
    "datasources": [
        {
            "access": "proxy",
            "name": "prometheus",
            "type": "prometheus",
            "url": "http://prometheus:9090", /* адрес service prometheus */
        }
    ]
}
```

**go-pg-crud_dashboard.json** приводить не буду, т.к. подробное описание можно посмотреть на [сайте grafana](https://grafana.com/grafana/dashboards/6671)

Применяем конфигурацию:

```console
# переходим в директорию grafana
$ cd grafana

# с помощью kustomize генерируем конфигурацию и применяем
$ kustomize build . | oc apply -f -
configmap/grafana-config created
configmap/grafana-dashboard-go-pg-crud created
configmap/grafana-dashboards created
configmap/grafana-datasources created
service/grafana created
deploymentconfig.apps.openshift.io/grafana created
route.route.openshift.io/grafana created

# проверяем наличе запущенных pod
$ oc get pod
NAME                  READY   STATUS              RESTARTS   AGE
grafana-1-deploy      1/1     Running             0          4s
grafana-1-pdjwh       0/1     ContainerCreating   0          1s
prometheus-1-deploy   0/1     Completed           0          53m
prometheus-1-pm2tn    1/1     Running             0          53m

# проверяем наличие configmaps
$ oc get cm | grep grafana
grafana-config                 1      30m
grafana-dashboard-go-pg-crud   1      30m
grafana-dashboards             1      30m
grafana-datasources            1      30m

# проверяем наличие route
$ oc get route | grep grafana
grafana      grafana-training-monitoring.apps.ocp-test.<domain_name>             grafana      3000                 None

# проверяем наличие service
$ oc get service | grep grafana
grafana      ClusterIP   172.30.101.198   <none>        3000/TCP   31m

```

Сервис grafana будет доступен по адресу http://grafana-training-monitoring.apps.ocp-test.<domain_name> . В интерфейсе grafana будет доступен datasource к prometheus server и добавлен dashboard.

## Alertmanager

**Alertmanager** - это инструмент для обработки оповещений, который устраняет дубликаты, группирует и отправляет оповещения соответствующему получателю.

Для запуска alertmanager необходимо создать deploymentconfig, в котором будет запущен образ docker.io/prom/alertmanager, и конфигурационные файлы:
* alertmanager.yml - основной файл конфигурации alertmanager

**alertmanager.yml**

```yaml
# блок глобальных переменных
global:
  resolve_timeout: 5m # если событие тригера не было обновлено в течении 5 минут, то оно считается неактуальным

# блок настройки оповещений
route:
  group_by: ['alertname'] # груповать события по label
  group_wait: 10s # ожидание перед отправкой событий
  group_interval: 10s # промежуток времени перед отправками событий
  repeat_interval: 120s # интервал повтора отправки событий
  receiver: 'alertmananger-bot' # чем будем отправлять, описание отправки в блоке receivers
receivers: # куда и как отправлять
- name: 'alertmananger-bot'
  webhook_configs: # используется webhook
  - send_resolved: true # отправлять уведомления со статусом resolved
    url: 'http://alertmanager-bot:8080' # отравляем оповещения в приложение, работающее с ботом telegram
```

Применяем конфигурацию:

```console
# переходим в директорию alertmanager
$ cd alertmanager

# с помощью kustomize генерируем конфигурацию и применяем
$ kustomize build . | oc apply -f -
configmap/alertmanager-config created
service/alertmanager created
deploymentconfig.apps.openshift.io/alertmanager created
route.route.openshift.io/alertmanager created

# проверяем наличие запущенных pod
$ oc get pod | grep alertmanager
alertmanager-1-c4wvr    1/1     Running     0          38s
alertmanager-1-deploy   0/1     Completed   0          41s

# проверяем создание configmaps
$ oc get cm | grep alertmanager
alertmanager-config            1      50s

# проверяем создание route
$ oc get route | grep alertmanager
alertmanager   alertmanager-training-monitoring.apps.ocp-test.<domain_name>          alertmanager   9093                 None

# проверяем создание service
$ oc get svc | grep alertmanager
alertmanager   ClusterIP   172.30.98.31     <none>        9093/TCP   57s
```

Сервис alertmanager будет доступен по адресу http://alertmanager-training-monitoring.apps.ocp-test.<domain_name>


## Alertmanager-bot

**Alertmanager-bot** - это сервис для отправки оповещений из alertmanager в telegram. Сайт проекта [github.com/metalmatze/alertmanager-bot](https://github.com/metalmatze/alertmanager-bot)

Перед запуском сервиса alertmanager необходимо [создать бота в Telegram](https://core.telegram.org/bots) и узнать свой или любой другой [id пользователя](https://messenge.ru/kak-uznat-id-telegram/), который будет являться администратором бота.

Далее необходимо заполнить secrets:
* admin - id пользователя telegram, который будет являться администратором
* token - id бота telegram

Предварительно admin и token необходимо конвертировать в base64 и добавить в secrets, например:

```console
$ echo 12345 | base64
MTIzNDUK
```

Для запуска alertmanager создаём deploymentconfig и secrets, также нам необходим service для доступа alertmanager к alertmanager-bot:

```console
# переходим в директорию alertmanager-bot
$ cd alertmanager-bot

# с помощью kustomize генерируем конфигурацию и применяем
$ kustomize build . | oc apply -f -
secret/alertmanager-bot created
service/alertmanager-bot created
deploymentconfig.apps.openshift.io/alertmanager-bot created

# проверяем наличие запущенных pod
oc get pod | grep alertmanager-bot
alertmanager-bot-1-deploy   0/1     Completed   0          53s
alertmanager-bot-1-zshl6    1/1     Running     0          50s

# проверяем наличие secrets
$ oc get secrets | grep alertmanager-bot
alertmanager-bot                  Opaque                                2      73s

# проверяем наличие service
$ oc get svc | grep alertmanager-bot
alertmanager-bot   ClusterIP   172.30.3.92      <none>        8080/TCP   88s
```

После успешного запуска pod проверяем корректность работы alertmanager-bot, например команда /help:

```
*/help*

*alertmanager_neoflex_training*

I'm a Prometheus AlertManager Bot for Telegram. I will notify you about alerts.
You can also ask me about my /status, /alerts & /silences

Available commands:
/start - Subscribe for alerts.
/stop - Unsubscribe for alerts.
/status - Print the current status.
/alerts - List all alerts.
/silences - List all silences.
/chats - List all users and group chats that subscribed.
```

## Проверка работы системы мониторинга и оповещений

После запуска всех сервисов убеждаемся что pod в статусе Running и доступны Prometheus server, Grafana, Alertmanager веб-интерфейсы:
*  http://prometheus-training-monitoring.apps.ocp-test.<domain_name>
*  http://grafana-training-monitoring.apps.ocp-test.<domain_name>
*  http://alertmanager-training-monitoring.apps.ocp-test.<domain_name>

```console
$ oc get pod | grep Running
alertmanager-1-6lvw6        1/1     Running     0          15h
alertmanager-bot-1-zshl6    1/1     Running     0          15h
grafana-1-pdjwh             1/1     Running     0          16h
prometheus-1-pm2tn          1/1     Running     0          17h
```

Также необходимо убедиться что метрики корректно собираются и отображаются. Проверяем раздел **Status -> Targets** в Prometheus, и dashboard в Grafana.

## Генерируем нагрузку

Для генерации нагрузки будем использовать [yandex-tank](https://github.com/yandex/yandex-tank). Запускаем как и рекомендовано в docker:

```console
$ docker run --entrypoint /bin/bash -v $(pwd):/var/loadtest -v $HOME/.ssh:/root/.ssh --net host -it direvius/yandex-tank
```

Для генерации нагрузки создаём файл load.yaml:

```yaml
phantom:
  address: go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name>:80 # [Target's address]:[target's port]
  writelog: all
  uris: # список url которые будем вызывать
    - /index.html
    - /book.html?id=10
    - /book.html?id=11
    - /book.html?id=12
  load_profile:
    load_type: rps # schedule load by defining requests per second
    schedule: const(1000, 10m) # starting from 1rps growing linearly to 1000rps during 10 minutes
  headers:
    - "[Host: go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name>]"
    - "[Connection: close]"
console:
  enabled: true # enable console output
telegraf:
  enabled: false # let's disable telegraf monitoring for the first time
```

и запускаем нагрузку:

```console
# yandex-tank -c load.yml
```

Из-за большого количества обращений к сервису go-pg-crud возрастёт количество потоков goroutine генерируемые приложением, на которые у нас в Prometheus server настроено правило:

```
...
            expr: go_goroutines{job="go-pg-crud"} > 100
...
```

В результате prometheus Server сгенерирует событие и отправит его в Alertmanager, а Alertmanager уже отправит сгенерированное событие в Alertmanager-bot. В результате в Telegram будет получено сообщение вида:

```
🔥 FIRING 🔥
HighGoroutine
Check server load!
Duration: 6 minutes 30 seconds
```

и по завершении нагрузки:

```
RESOLVED
HighGoroutine
Check server load!
Duration: 3 minutes 30 seconds
Ended: 5 seconds
```



