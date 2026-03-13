projectc-custodial-wallet
===================


## Framework

`projectc-custodial-wallet`框架的核心就是pkg包，下面主要针对该包结构进行描述：

```bash
pkg/
├── config
│   ├── config.go
│   ├── key.go
│   ├── model.go
│   └── opt_defs.go
├── controller
│   ├── ping.go
│   ├── todo.go
│   └── version.go
├── log
│   └── log.go
├── middleware
│   ├── basic_auth_middleware.go
├── models
│   └── common.go
├── route
│   └── routes.go
├── service
│   └── todo.go
├── store
└── util
```

* config：主要用于配置文件，实现：文件+环境变量+命令行参数读取
* controller: 对应MVC中controller，调用service中的接口进行实际处理，自己只做数据校验与拼接
* service: 负责主要的逻辑实现
* log: 日志模块，实现：模块名(文件名)+函数名+行数+日志级别
* middleware: 中间件，负责通用的处理，例如：鉴权
* models: 对应MVC中的model
* route: gin路由
* store: 存储模块，可以添加MySQL、Redis等
* util: 通用的库函数

## Usage

* step1 - 替换项目名称

  实际使用中，通常需要将`projectc-custodial-wallet`替换成业务需要的后台server名称，可以执行如下命令：

  ```bash
  $ grep -rl projectc-custodial-wallet . | xargs sed -i 's/projectc-custodial-wallet/youapiserver/g' 
  ```
  
* step2 - 开发业务controller和service

  框架中已经集成了一个示例(todo)：
  
  ```go
  // controller(pkg/controller/todo.go)
  type ToDoController interface {
  	GetToDo(c *gin.Context)
  }
  
  // service(pkg/service/todo.go)
  type ToDoService interface {
  	Get()
  }
  ```
  
  我们需要按照自身业务需求开发todo(替换成任意功能)的controller和service逻辑。另外你也可以参考todo添加其它功能对应的controller和service
   
* step3 - 启动服务  

  可以直接启动运行服务，如下：

  ```bash
  $ bash hack/start.sh
  ```
  
  也可以在Kubernetes集群中启动服务，如下：
  
  ```bash
  # generated image
  $ make dockerfiles.build
  # retag and push to your docker registry
  $ docker tag guyuxiang/projectc-custodial-wallet:v0.1.0 xxx/guyuxiang/projectc-custodial-wallet:v0.1.0
  $ docker push xxx/guyuxiang/projectc-custodial-wallet:v0.1.0
  # Update the deployment to use the built image name
  $ sed -i 's|REPLACE_IMAGE|xxx/guyuxiang/projectc-custodial-wallet:v0.1.0|g' hack/deploy/deployment.yaml
  # create service 
  $ kubectl apply -f hack/deploy/service.yaml
  # create deployment
  $ kubectl apply -f hack/deploy/deployment.yaml
  ```

## Refs

* [dump-goroutine-stack-traces](https://colobu.com/2016/12/21/how-to-dump-goroutine-stack-traces/)
