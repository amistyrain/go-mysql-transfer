#!/bin/bash

# Go-MySQL-Transfer Docker Compose 启动脚本

set -e

echo "=== Go-MySQL-Transfer Docker Compose 部署脚本 ==="

# 检查必要文件
check_files() {
    echo "检查必要文件..."
    
    if [ ! -f "docker-compose.yml" ]; then
        echo "错误: docker-compose.yml 文件不存在"
        exit 1
    fi
    
    if [ ! -f "Dockerfile" ]; then
        echo "错误: Dockerfile 文件不存在"
        exit 1
    fi
    
    if [ ! -f "app.yml" ]; then
        echo "错误: 配置文件不存在，请创建 app.yml"
        exit 1
    fi
}

# 创建必要目录
create_directories() {
    echo "创建必要目录..."
    mkdir -p store/log
    mkdir -p store/db
    mkdir -p lua
    
    # 设置权限
    if [ -d "store" ]; then
        chmod -R 755 store
    fi
}

# 检查Docker和Docker Compose
check_docker() {
    echo "检查Docker环境..."
    
    if ! command -v docker &> /dev/null; then
        echo "错误: Docker 未安装或未在PATH中"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        echo "错误: Docker Compose 未安装或未在PATH中"
        exit 1
    fi
    
    # 检查Docker是否运行
    if ! docker info &> /dev/null; then
        echo "错误: Docker 服务未启动"
        exit 1
    fi
}

# 停止现有服务
stop_services() {
    echo "停止现有服务..."
    docker-compose down 2>/dev/null || true
}

# 构建和启动服务
start_services() {
    echo "构建并启动服务..."
    
    # 拉取基础镜像 (简化版本无需拉取外部镜像)
    
    # 构建应用镜像
    echo "构建应用镜像..."
    docker-compose build go-mysql-transfer
    
    # 启动服务
    echo "启动服务..."
    docker-compose up -d
    
    # 等待服务启动
    echo "等待服务启动..."
    sleep 10
}

# 检查服务状态
# check_services() {
    # echo "检查服务状态..."
    # docker-compose ps
    
    # # 健康检查
    # echo "进行健康检查..."
    # sleep 5
    
    # if curl -s http://localhost:8060 > /dev/null; then
    #     echo "✅ Web管理界面正常"
    # else
    #     echo "❌ Web管理界面异常，请检查配置和外部服务连接"
    # fi
# }

# 显示日志
show_logs() {
    echo ""
    echo "=== 最近日志 ==="
    docker-compose logs --tail=20 go-mysql-transfer
    
    echo ""
    echo "持续查看日志请运行: docker-compose logs -f go-mysql-transfer"
    echo "停止服务请运行: docker-compose down"
}

# 主流程
main() {
    check_docker
    check_files
    create_directories
    stop_services
    start_services
    # check_services
    show_logs
}

# 处理参数
case "${1:-start}" in
    start)
        main
        ;;
    stop)
        echo "停止所有服务..."
        docker-compose down
        ;;
    restart)
        echo "重启服务..."
        docker-compose restart
        ;;
    logs)
        docker-compose logs -f go-mysql-transfer
        ;;
    status)
        docker-compose ps
        ;;
    clean)
        echo "清理所有容器和数据..."
        docker-compose down -v
        docker system prune -f
        ;;
    *)
        echo "使用方法: $0 {start|stop|restart|logs|status|clean}"
        echo ""
        echo "  start   - 启动所有服务 (默认)"
        echo "  stop    - 停止所有服务"
        echo "  restart - 重启服务"
        echo "  logs    - 查看日志"
        echo "  status  - 查看服务状态"
        echo "  clean   - 清理容器和数据"
        exit 1
        ;;
esac
