# CI\\CD в OpenShift с помощью Gitlab CI

## Обзор окружения

Для показа примера интеграции OpenShift и Gitlab CI развёрнуто следующее окружение:

* ВМ **bastion**
  - bind
  - haproxy
* Кластер **OpenShift**
* ВМ **openshift-infra**
  - Gitlab CI
  - Nexus
  - Postgres
* ВМ **openshift-build**
  - git
  - vi\\vim
  - oc
  - docker
  - go

![](/images/img_1.png)

#### **ВМ bastion**

**Роль:** bastion хост для установки кластера OpenShift и bind в качестве DNS сервера.

#### ВМ **openshift-infra**

**Роль:** Запущены сервисы Gitlab CI, Nexus, Postgres

Сервисы запущены в docker.

#### ВМ **openshift-build**

**Роль:** Хост для сборки приложений и работы с OpenShift.

#### **Кластер OpenShift**

```console
# получить список нод и их статус
$ oc get nodes
NAME STATUS ROLES AGE VERSION
master-0.ocp-test.<domain_name> Ready master,worker 39d v1.18.3+6c42de8
master-1.ocp-test.<domain_name> Ready master,worker 39d v1.18.3+6c42de8
master-2.ocp-test.<domain_name> Ready master,worker 39d v1.18.3+6c42de8
worker-0.ocp-test.<domain_name> Ready worker 39d v1.18.3+6c42de8
worker-1.ocp-test.<domain_name> Ready worker 39d v1.18.3+6c42de8

# получить версию кластера
$ oc get clusterversion
NAME VERSION AVAILABLE PROGRESSING SINCE STATUS
version 4.5.8 True False 39d
```

Изменены параметры insecure registry(отключает проверку сертификатов и добавляет registry в доверенные):

```console
# редактируем параметры кластера
$ oc edit image.config.openshift.io/cluster
...
spec:
registrySources:
insecureRegistries:
- openshift-infra.test.<domain_name>:5000
...
```

Создан namespace для деплоя приложения:

```console
# создаём новый проект с именем go-pg-crud
$ oc new-project go-pg-crud
```

Создан secret docker-registry для доступа к репозиторию nexus:

```console
# создаём secret типа docker-registry с именем nexus-pull
$ oc create secret docker-registry nexus-pull --docker-server=openshift-infra.test.<domain_name>:5000 --docker-username=gitlab --docker-password=пароль_пользователя --docker-email=gitlab@test.<domain_name>
```

Неоходимо пользователю default в проекте go-pg-crud предоставить права на pull из репозитория:

```console
# предоставляем пользователю default права на pull по ранее созданному secret nexus-pull
$ oc secrets link default nexus-pull --for=pull
```

Также создан пользователь, для доступа к проекту. Выданы права cluster-admin(так делать нельзя, но с RBAC сейчас времени разбираться нет, сразу в прод :rage2: ):

```console
# создаём service account
$ oc create sa go-pg-crud

# предоставляем пользовалю права: добавляем роль cluster admin для пользователя go-pg-crud в проекте go-pg-crud
$ oc adm policy add-role-to-user cluster-admin system:serviceaccount:go-pg-crud:go-pg-crud
```

После создания пользователя, будут сгененрированы secrets с токенами. Один из токенов нам будет необходим для доступа к api из Gitlab CI.

```console
# получить список secrets
$ oc get secrets | grep go-pg-crud
go-pg-crud-dockercfg-klr2q kubernetes.io/dockercfg 1 13h
go-pg-crud-token-7v6nm kubernetes.io/service-account-token 4 13h
go-pg-crud-token-wq9n7 kubernetes.io/service-account-token 4 13h

# подробный вывод содержимого secrets
$ oc describe secrets go-pg-crud-token-7v6nm
...
token: eyJhbGciOiJSUzI1...
...
```

#### Nexus

Nexus используется как docker registry. Создан docker репозиторий, доступный на порту 5000:

![](/images/img_2.png)

Также добавлен пользователь gitlab c правами nx-admin(точно также как с RBAC, нет времени, лей на прод):

![](/images/img_3.png)

И разрешён логин в nexus по Docker Bearer Token:

![](/images/img_4.png)

#### Gitlab CI

Gitlab CI как основной инструмент для CI\\CD. Также запущен runner для запуска выполняемых задач в pipeline.

Скрипт регистрации runner можно найти в приложении.

В Gitlab CI создан репозиторий, в котором загружен код проекта:

![](/images/img_5.png)

После создания репозитория необходимо добавить проект в git, для этого на хосте openshift-build в директории с проектом:

```console
# инициализируем репозиторий
$ git init
# указываем ссылку на репозиторий
$ git remote add origin ssh://git@openshift-infra.test.<domain_name>:2222/ashtodin/go-pg-crud-build.git
# добавляем все файлы в директории в индекс (staging area)
$ git add .
# делаем сохранение индекса с сообщением "Initial commit" 
$ git commit -m "Initial commit"
# переносим данные в репозиторий git в ветку master
$ git push -u origin master
```

Для проекта добавляем runner. Предаврительно runner должен быть запущен и зарегистрирован в GitLab CI.

![](/images/img_6.png)

Для пользователя разрешён доступ по ssh к репозиторию - в GitLab CI добавлен ssh ключ в **User settings -> SSH Keys**.

#### Postgress

Приложение работает с БД, поэтому отдельно запущен postgres на хосте openshift-infra.

Создана база данных **books_database** и таблица books.

#### ВМ openshift-build

Установлены утилиты docker, oc, git, vim, go

*   [https://docs.docker.com/engine/install/centos/](https://docs.docker.com/engine/install/centos/)
*   [https://docs.openshift.com/container-platform/4.6/cli\_reference/openshift\_cli/getting-started-cli.html](https://docs.openshift.com/container-platform/4.6/cli_reference/openshift_cli/getting-started-cli.html)
*   yum -y install git vim go

Конфигурация docker для доступа к репозиторию nexus, т.к. по умолчанию docker считает все docker registry безопасными и проверяет сертификат, необходимо добавить в /etc/docker/daemon.json:

```json
{
"insecure-registries" : ["openshift-infra.test.<domain_name>:5000"]
}

# после изменения настроек необходимо перезапустить сервис docker: systemctl restart docker
```

Проверить доступность docker registry:

```console
$ docker login openshift-infra.test.<domain_name>:5000 -u gitlab
Password: 
WARNING! Your password will be stored unencrypted in /root/.docker/config.json.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store

Login Succeeded
```

Возможно будет необходимым добавить правила для firewalld (он включен по умолчанию в Centos):

```console
# firewall-cmd --zone=public --add-masquerade --permanent # для корректной работы docker
# firewall-cmd --zone=public --permanent --add-port=8080/tcp # для доступа к нашему приложения по 8080
# firewall-cmd --reload # обновление конфигурации
```

## Сборка и деплой приложения

#### Готовим Dockerfile

Клонируем репозиторий с кодом:

```console
$ git clone ssh://git@openshift-infra.test.<domain_name>:2222/ashtodin/openshift-deploy.git
```

Собираем приложение, пишем Dockerfile и собираем образ:

```dockerfile
# Dockerfile
# Базовый образ для сборки приложения
FROM golang:1.10-alpine3.7
# устанавливаем рабочую директорию
WORKDIR /go/src/app
# копируем содержимое текущей директории в /go/src/app ораза
COPY . .
# устанавлием в образ curl и git
RUN apk add --no-cache curl git
# скачиваем зависимости для приложения
RUN go get github.com/lib/pq
# скачиваем зависимости для приложения
RUN go get github.com/prometheus/client_golang/prometheus/promhttp
# запускаем сборку приложения
RUN go build -o go-pg-crud
# указываем порт рабочего приложения
EXPOSE 8080/tcp
# указываем путь к собранному ранее приложению, которое docker будет выполняться при запуск образа
ENTRYPOINT ["/go/src/app/go-pg-crud"]
```

Сборка образа:

```console
# собираем образ с тегом go-pg-crud
$ docker build -t go-pg-crud .
Sending build context to Docker daemon 543.7kB
Step 1/9 : FROM golang:1.10-alpine3.7
---> 0c7ca152fa16
....
Step 9/9 : ENTRYPOINT ["/go/src/app/go-pg-crud"]
---> Running in b46a51f1687a
Removing intermediate container b46a51f1687a
---> 0e1b32744b13
Successfully built 0e1b32744b13
Successfully tagged go-pg-crud:latest

# запускаем собранный образ и пробрасываем порт 8080, на котором работает приложение
$ docker run -p 8080:8080 go-pg-crud:latest 
```

После запуска образа приложение доступно по ссылке: [http://openshift-build.test.<domain_name>:8080/](http://openshift-build.test.<domain_name>:8080/)

#### Пишем pipeline для Gitlab CI

В корне репозитория необходимо создать файл .gitlab-ci.yml со следующим содержимым:

```yaml
# обьявляем переменные
variables:
PACKAGE_PATH: /go/src/gitlab.com/go-pg-crud
OPENSHIFT_PROJECT: go-pg-crud

# указываем, что при запуске pipeline необходимо запустить docker образ docker:dind
services:
- name: docker:dind
command: ["--insecure-registry=$DOCKER_REGISTRY"]

# список шагов в pipeline
stages:
- test
- build
- deploy

# функция, при выполнении которой создаётся рабочая директория из переменной PACKAGE_PATH и ссылка на эту директорию из переменной CI_PROJECT_DIR. Необходимо для корректной сборки проекта, костыль в общем.
.anchors:
- &inject-gopath
mkdir -p $(dirname ${PACKAGE_PATH})
&& ln -s ${CI_PROJECT_DIR} ${PACKAGE_PATH}
&& cd ${PACKAGE_PATH}

# шаг запуска тестов
# скачиваем зависимости запускаем тесты
Run test:
stage: test
image: golang:1.10-alpine3.7
before_script:
- apk add --no-cache curl git
- export REVISION="$(git rev-parse --short HEAD)" && echo $REVISION | tr -d '\n' > variables
- wget https://github.com/golang/dep/releases/download/v0.5.4/dep-linux-amd64 -O /go/bin/dep
- chmod +x /go/bin/dep
- *inject-gopath
script:
- dep init
- dep ensure -v -vendor-only
- go test .
- cat variables
artifacts:
name: "vendor-$CI_PIPELINE_ID"
paths:
- vendor/
- variables

# шаг запуска сборки приложения
# собираем приложение и загружаем в nexus
Build app:
stage: build
before_script:
- export REVISION="$(cat variables | tr -d '\n')"
- docker login -u $DOCKER_REGISTRY_USER -p $DOCKER_REGISTRY_PASSWORD $DOCKER_REGISTRY
dependencies:
- Run test
image: docker:19.03.13-git
script:
- docker build -t $DOCKER_REGISTRY/go-pg-crud/go-pg-crud:${REVISION} .
- docker push $DOCKER_REGISTRY/go-pg-crud/go-pg-crud:${REVISION}

# шаг подготовки yaml манифестов для разворачивания в OpenShift
Create manifest:
stage: build
image: traherom/kustomize-docker
dependencies:
- Run test
before_script:
- export REVISION="$(cat variables | tr -d '\n')"
script:
- kustomize edit set image $DOCKER_REGISTRY/go-pg-crud/go-pg-crud:latest=$DOCKER_REGISTRY/go-pg-crud/go-pg-crud:${REVISION}
- kustomize build .
- kustomize build . > kustomize_deploy.yaml
artifacts:
paths:
- kustomize/kustomize_deploy.yaml

# шаг разворачивания приложения в OpenShift
Deploy:
stage: deploy
image: widerin/openshift-cli
dependencies:
- Create manifest
before_script:
- oc login --token=$OPENSHIFT_TOKEN --server=$OPENSHIFT_API --insecure-skip-tls-verify=true
- oc project $OPENSHIFT_PROJECT
script:
- oc create -f kustomize/kustomize_deploy.yaml || true
- oc replace -f kustomize/kustomize_deploy.yaml
```

Т.к. в pipeline используются переменные, то необходимо их добавить в проект:

* DOCKER\_REGISTRY = openshift-infra.test.<domain_name>:5000    
* DOCKER\_REGISTRY\_PASSWORD = пароль пользователя, в данном случае пароль ранее созданного пользователя gitlab  
* DOCKER\_REGISTRY\_USER = gitlab    
* OPENSHIFT\_API = [https://api.ocp-test.<domain_name>:6443](https://api.ocp-test.<domain_name>:6443)  
* OPENSHIFT\_TOKEN = токен пользователя OpenShift - go-pg-crud  
    

![](/images/img_7.png)

Далее необходимо изменить Dockerfile, который мы будем использовать для сборки приложения. Т.к. зависимости для приложения используются несколько раз - на шаге тестирования (Run test) и на шаге сборки образа (Build app), то правильный вариант это провести загрузку зависимостей на шаге тестирования приложения, а на этапе сборки образа (Build app) использовать уже выкачаные зависимости. Использовать будем утилиту dep, которая самостоятельно получает необходимые зависимости для сборки проекта. Итоговый Dockerfile будет выглядеть следующим образом:

```dockerfile
# Dockerfile
FROM golang:1.10-alpine3.7

WORKDIR /go/src/app

COPY . .

RUN go build -o go-pg-crud

EXPOSE 8080/tcp

ENTRYPOINT ["/go/src/app/go-pg-crud"]
```
  

#### Готовим yaml манифесты для деплоя в OpenShift:

Для управления yaml манифестами будем использовать утилиту kustomize. Нам при сборке обновлённого проекта необходимо менять тег docker образа, т.к. в pipeline мы при каждой сборке приложения сохраняем id коммита в качестве тега образа. Подробнее про [https://kustomize.io/](https://kustomize.io/) .

![](/images/img_8.png)

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

![](/images/img_10.png)


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
