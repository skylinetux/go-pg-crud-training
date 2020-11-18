# Мониторинг: Prometheus, Grafana, Alertmanager

Цель данного проекта в том, чтобы показать как работают и взаимодействуют Prometheus, Grafana и Alertmanager. Если всё пойдёт хорошо, то в результате получим работающую систему мониторинга с оповещениями пользователей в Telegram.

## Обзор окружения

Для организации примера мониторинга приложения развёрнуто тоже окружение что и в [примере с CI\\CD](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd). В данном случае нам понадобится только **кластер OpenShift** и [приложение](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd/application) которое будем заводить в мониторинг, и конечно же хост, с которого будем производить установку и настройку стека мониторинга.

* Кластер **OpenShift**
* ВМ **openshift-infra**
  - Postgres
* ВМ **openshift-build**
  - git
  - vi\\vim
  - oc
  - kustomize

#### ВМ **openshift-infra**

**Роль:** Запущены сервисы Gitlab CI, Nexus, Postgres. В данном примере используется только БД Postgres для корректной работы [приложения](https://github.com/skylinetux/neoflex_training/tree/master/ci_cd/application).

Сервисы запущены в docker.

#### **Кластер OpenShift**

```console
# получить список нод и их статус
$ oc get nodes
NAME                              STATUS   ROLES            AGE   VERSION
master-0.ocp-test.neoflex.local   Ready    master,worker    68d   v1.18.3+45b9524
master-1.ocp-test.neoflex.local   Ready    master,worker    68d   v1.18.3+45b9524
master-2.ocp-test.neoflex.local   Ready    master,worker    68d   v1.18.3+45b9524
worker-0.ocp-test.neoflex.local   Ready    compute,worker   68d   v1.18.3+45b9524
worker-1.ocp-test.neoflex.local   Ready    compute,worker   68d   v1.18.3+45b9524

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


## Общая схема работы мониторинга

![](/monitoring/images/img_1.png)

**Prometheus** выступает в роли хранилища метрик, которые мы будем получать из приложения. Также в Promethes будем генерировать события для Alertmanager.
В **Grafana** будем создавать дашборды для визуализации метрик.
**Alertmanager** работает в качестве системы оповещения. Получая события из Prometheus, обрабытывая их, Alertmanager будет передавать данные в Alertmanager-bot.
**Alertmanager-bot** система отправки оповещений в Telegram. Сам проект доступен в [github.com/metalmatze/alertmanager-bot](https://github.com/metalmatze/alertmanager-bot).
**Метрики приложения** доступны по 

**Цель:** получить работающую систему мониторинга с оповещениями пользователей.

## Подготовка окружения для в OpenShift

Для начала необходимо подготовить окружение для запуска системы мониторинга. Необходимо:
* Создать namespace
* Создать учётную запись с правами доступа к метрикам приложений

Создаём namespace с именем training-monitoring и serviceaccount(учётная запись) с именем metricsexporter. Для metricsexporter предоставляем роли cluster-reade и view для доступа к метрикам сервисов. Под учётной записью metricsexporter будет запущен только pod с prometheus server. Всё остальные pod будут использовать учётную запись default.

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

Важно отметить, что адрес сервиса alertmanager:9093 указан по имени service, и аналогично go-pg-crud.go-pg-crud.svc:80 - serivce с именем go-pg-crud в namespace go-pg-crud. Доступ к service в других namespace мы предоставили выше для учётной записи metricsexporter. Обращение через service в нашем случае удобнее - мы собираем метрики внутри сети OpenShift, но возможен и вариант указания route необходимых сервисов, но тогда обращения к метрикам будут дополнительно проходить через балансировщик OpenShift, что не совсем рационально, но для сервисов вне OpenShift такой вариант является основным.

Также создаём route и service для доступа к prometheus server. Полностью создание prometheus service в OpenShift будет выглядеть следующим образом.

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
# проверяем создание configmaps
$ oc get cm
NAME                DATA   AGE
prometheus-config   1      13m
prometheus-rules    1      13m
# проверяем создание route
$ oc get route
NAME         HOST/PORT                                                    PATH   SERVICES     PORT   TERMINATION   WILDCARD
prometheus   prometheus-training-monitoring.apps.ocp-test.<domain_name>         prometheus   9090                 None
# проверяем создание service
$ oc get svc
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
prometheus   ClusterIP   172.30.136.92   <none>        9090/TCP   13m
```

Сервис prometheus будет доступен по адресу http://prometheus-training-monitoring.apps.ocp-test.<domain_name>

## Grafana

**Grafana** - это платформа с открытым исходным кодом для визуализации, мониторинга и анализа данных. 

Для запуска grafana необходимо также создать deploymentconfig, в котором будет запущен образ docker.io/grafana/grafana, и конфигурационные файлы:
* grafana.ini - основной файл конфигурации grafana
* go-pg-crud_dashboard.json - заранее подготовленный дашборд для мониторинга приложения на golang, за основу взят [dashboard с сайта grafana](https://grafana.com/grafana/dashboards/6671)
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
http_port = 3000 # grafana будет запущена на 3000 порту
```

**dashboards.yaml**

```json
{
    "apiVersion": 1,
    "providers": [
        {
            "folder": "",
            "name": "0",
            "options": { # путь хранения дашбордов, которые будут использоваться как дочерние
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
            "url": "http://prometheus:9090", # адрес service prometheus
        }
    ]
}
```

**go-pg-crud_dashboard.json** приводить не буду, т.к. подробное описание можно посмотреть на [сайте grafana](https://grafana.com/grafana/dashboards/6671)

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
# проверяем создание configmaps
$ oc get cm | grep grafana
grafana-config                 1      30m
grafana-dashboard-go-pg-crud   1      30m
grafana-dashboards             1      30m
grafana-datasources            1      30m
# проверяем создание route
$ oc get route | grep grafana
grafana      grafana-training-monitoring.apps.ocp-test.neoflex.local             grafana      3000                 None
# проверяем создание service
$ oc get service | grep grafana
grafana      ClusterIP   172.30.101.198   <none>        3000/TCP   31m

```

Сервис grafana будет доступен по адресу http://grafana-training-monitoring.apps.ocp-test.<domain_name>

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
  group_by: ['alertname'] # груповать по label
  group_wait: 10s # ожидание перед отправкой оповещений
  group_interval: 10s # промежуток времени перед отправками оповещений
  repeat_interval: 120s # интервал повтора оповещения
  receiver: 'alertmananger-bot' # куда и как отправлять, блок ниже
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
# проверяем наличе запущенных pod
$ oc get pod | grep alertmanager
alertmanager-1-c4wvr    1/1     Running     0          38s
alertmanager-1-deploy   0/1     Completed   0          41s
# проверяем создание configmaps
$ oc get cm | grep alertmanager
alertmanager-config            1      50s
# проверяем создание route
$ oc get route | grep alertmanager
alertmanager   alertmanager-training-monitoring.apps.ocp-test.<domain_name>          alertmanager   9093                 None
$ проверяем создание service
# oc get svc | grep alertmanager
alertmanager   ClusterIP   172.30.98.31     <none>        9093/TCP   57s
```

Сервис alertmanager будет доступен по адресу http://alertmanager-training-monitoring.apps.ocp-test.<domain_name>








































#### Готовим yaml манифесты для деплоя в OpenShift:

Для управления yaml манифестами будем использовать утилиту kustomize. Нам при сборке обновлённого проекта необходимо менять тег docker образа, т.к. в pipeline мы при каждой сборке приложения сохраняем id коммита в качестве тега образа. Подробнее про [https://kustomize.io/](https://kustomize.io/) .

![](/ci_cd/images/img_8.png)

*kustomization.yaml*:

```yaml
resources:
- 01-deploymentconfig.yaml
- 02-service.yaml
- 03-route.yaml
```

exclamation Один из вариантов сформировать deploymentconfig, service и route - развернуть приложение в OpenShift и выгрузить yaml через **oc get _ресурс_ --export**

*01-deploymentconfig.yaml*:

```yaml
apiVersion: apps.openshift.io/v1
kind: DeploymentConfig
metadata:
name: go-pg-crud
spec:
replicas: 1
revisionHistoryLimit: 10
selector:
app: go-pg-crud
strategy:
template:
metadata:
labels:
app: go-pg-crud
spec:
containers:
- image: openshift-infra.test.<domain_name>:5000/go-pg-crud/go-pg-crud:latest
imagePullPolicy: Always
name: go-pg-crud
ports:
- containerPort: 8888
protocol: TCP
resources: {}
terminationMessagePath: /dev/termination-log
terminationMessagePolicy: File
dnsPolicy: ClusterFirst
restartPolicy: Always
schedulerName: default-scheduler
securityContext: {}
terminationGracePeriodSeconds: 30
```

*02-service.yaml*:

```yaml
apiVersion: v1
kind: Service
metadata:
name: go-pg-crud
spec:
clusterIP: 172.30.115.42
ports:
- port: 80
protocol: TCP
targetPort: 8080
selector:
app: go-pg-crud
sessionAffinity: None
type: ClusterIP
```

*03-route.yaml*:

```yaml
apiVersion: v1
items:
- apiVersion: route.openshift.io/v1
kind: Route
metadata:
annotations:
openshift.io/host.generated: "true"
name: go-pg-crud
spec:
host: go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name>
port:
targetPort: 8080
to:
kind: Service
name: go-pg-crud
weight: 100
wildcardPolicy: None
kind: List
```

На этапе сборки приложения(Create manifest) будем выполнять **kustomize edit set image**, где будем менять тег образа и **kustomize build** для формирования итогового манифеста.

#### Запускаем pipeline и проверяем

Все наши именения переносим в git и проверяем работу pipeline

```console
$ git add .
$ git commit -m 'Update project'
$ git push
```

В Gitlab CI должен запуститься pipeline:

![](/ci_cd/images/img_10.png)


После успешного выполения pipeline проверяем ресурсы в OpenShift:

```console
# получаем список pod в проекте go-pg-crud
$ oc get pod -n go-pg-crud
NAME READY STATUS RESTARTS AGE
go-pg-crud-1-deploy 0/1 Completed 0 15h
go-pg-crud-1-4v2rj 1/1 Running 0 14h

# получаем список service в проекте go-pg-crud
$ oc get svc -n go-pg-crud
NAME TYPE CLUSTER-IP EXTERNAL-IP PORT(S) AGE
go-pg-crud ClusterIP 172.30.115.42 <none> 80/TCP 15h

# получаем список route в проекте go-pg-crud
$ oc get route -n go-pg-crud
NAME HOST/PORT PATH SERVICES PORT TERMINATION WILDCARD
go-pg-crud go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name> go-pg-crud 8080 None
```

Приложение будет доступно по ссылке [http://go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name>](http://go-pg-crud-go-pg-crud.apps.ocp-test.<domain_name>)
